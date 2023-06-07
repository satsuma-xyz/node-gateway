package route

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"net/http"

	redis "github.com/go-redis/cache/v9"
	"github.com/satsuma-data/node-gateway/internal/cache"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"go.uber.org/zap"
)

type RequestExecutor struct {
	httpClient client.HTTPClient
	logger     *zap.Logger
	cache      *cache.RPCCache
}

type OriginError struct {
	err error
}

func (e *OriginError) Error() string {
	return fmt.Sprintf("error making request to origin: %v", e.err)
}

func (r *RequestExecutor) routeToConfig(
	ctx context.Context,
	requestBody jsonrpc.RequestBody,
	configToRoute *config.UpstreamConfig,
) (jsonrpc.ResponseBody, *http.Response, error) {
	bodyBytes, err := requestBody.Encode()
	if err != nil {
		r.logger.Error("Could not serialize request.", zap.Any("request", requestBody), zap.Error(err))
		return nil, nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	if err != nil {
		r.logger.Error("Could not create new http request.", zap.Any("request", requestBody),
			zap.String("upstreamID", configToRoute.ID), zap.Error(err))
		return nil, nil, err
	}

	httpReq.Header.Set("content-type", "application/json")

	if configToRoute.BasicAuthConfig.Username != "" && configToRoute.BasicAuthConfig.Password != "" {
		encodedCredentials := base64.StdEncoding.EncodeToString([]byte(configToRoute.BasicAuthConfig.Username + ":" + configToRoute.BasicAuthConfig.Password))
		httpReq.Header.Set("Authorization", "Basic "+encodedCredentials)
	}

	var (
		respBody jsonrpc.ResponseBody
		resp     *http.Response
	)

	_, isSingleRequestBody := requestBody.(*jsonrpc.SingleRequestBody)

	if r.cache != nil && r.cache.ShouldCacheMethod(requestBody.GetMethod()) && isSingleRequestBody {
		// In case of unknown caching errors, the httpReq might get used twice.
		// We must clone the httpReq otherwise the body will already be closed on the second request.
		respBody, resp, err = r.retrieveOrCacheRequest(cloneRequest(httpReq), requestBody, configToRoute)
		if err != nil {
			originError, _ := err.(*OriginError)
			// An OriginError indicates a cache miss and request failure to origin.
			// We want this error to bubble up.
			// Any other errors should be unhandled caching errors.
			// In these cases we default back to the "non-caching" behavior.
			if originError != nil {
				r.logger.Warn("caching error making request to origin", zap.Error(err), zap.Any("request", requestBody))
				return nil, nil, originError
			}
		} else {
			return respBody, resp, nil
		}
	}

	respBody, resp, err = r.getResponseBody(httpReq, requestBody, configToRoute)
	if err != nil {
		return nil, nil, err
	}

	return respBody, resp, nil
}

func (r *RequestExecutor) retrieveOrCacheRequest(httpReq *http.Request, requestBody jsonrpc.RequestBody, configToRoute *config.UpstreamConfig) (jsonrpc.ResponseBody, *http.Response, error) {
	var (
		respBody     jsonrpc.ResponseBody
		resp         *http.Response
		cachedResult json.RawMessage
	)

	singleRequestBody := requestBody.GetSubRequests()[0]
	// Even if the cache is down, redis-cache will route to the origin
	// properly without returning an error.
	err := r.cache.Once(&redis.Item{
		Key:   r.cache.GetKeyFromRequestBody(singleRequestBody),
		Value: &cachedResult,
		TTL:   cache.DefaultTTL,
		Do: func(*redis.Item) (interface{}, error) {
			var originError error
			respBody, resp, originError = r.getResponseBody(httpReq, requestBody, configToRoute) //nolint:bodyclose // linter bug

			// On cache miss, the request to origin fails.
			// respBody and resp are nil and nothing should be cached.
			// We should "fail out" of the outer function.
			if originError != nil {
				return nil, originError
			}
			defer resp.Body.Close()
			// On cache miss, the request to origin succeeds but Error is set on responseBody.
			// respBody and resp are not nil and nothing should be cached.
			// respBody and resp should be returned in the outer function.
			singleResponseBody := respBody.GetSubResponses()[0]
			if singleResponseBody.Error != nil {
				return nil, fmt.Errorf("error found, not caching: %v", singleResponseBody.Error)
			}
			return &singleResponseBody.Result, nil
		},
	})

	// A cache hit or the request to origin failed.
	if resp == nil && respBody == nil {
		// Request to origin failed, we should fail out here.
		if err != nil {
			return nil, nil, err
		}
		// Cache hit, fill in id and jsonrpc in the respBody to match the request.
		r.logger.Debug("cache hit", zap.Any("request", requestBody))

		resp = &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(new(bytes.Buffer)),
		}
		respBody = &jsonrpc.SingleResponseBody{
			ID:      *singleRequestBody.ID,
			JSONRPC: singleRequestBody.JSONRPCVersion,
			Result:  cachedResult,
		}
	}

	return respBody, resp, nil
}

func (r *RequestExecutor) getResponseBody(httpReq *http.Request, requestBody jsonrpc.RequestBody, configToRoute *config.UpstreamConfig) (jsonrpc.ResponseBody, *http.Response, error) {
	resp, err := r.httpClient.Do(httpReq)

	if err != nil {
		r.logger.Error("Error encountered when executing request.", zap.Any("request", requestBody),
			zap.String("upstreamID", configToRoute.ID), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, &OriginError{err}
	}
	defer resp.Body.Close()

	respBody, err := jsonrpc.DecodeResponseBody(resp)

	if err != nil {
		r.logger.Warn("Could not deserialize response.", zap.Any("request", requestBody),
			zap.String("upstreamID", configToRoute.ID), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, &OriginError{err}
	}

	r.logger.Debug("Successfully routed request to upstream.", zap.String("upstreamID", configToRoute.ID), zap.Any("request", requestBody), zap.Any("response", respBody))

	return respBody, resp, nil
}

func cloneRequest(r *http.Request) *http.Request {
	r2 := &http.Request{}
	*r2 = *r

	var b bytes.Buffer
	_, err := b.ReadFrom(r.Body)

	if err != nil {
		panic(err)
	}

	r.Body = io.NopCloser(&b)
	r2.Body = io.NopCloser(bytes.NewReader(b.Bytes()))

	return r2
}
