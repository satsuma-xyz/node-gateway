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
		// We can not send the same http.Request twice.
		// In cache of a cache miss and the request to origin fails.
		respBody, resp, err = r.retrieveOrCacheRequest(cloneRequest(httpReq), requestBody, configToRoute)
		if err != nil {
			r.logger.Warn("caching error", zap.Error(err), zap.Any("request", requestBody))
		}
	}

	// If resp is nil, then either caching is disabled or caching failed.
	// We want to send the request to the origin in this case.
	if resp == nil {
		respBody, resp, err = r.getResponseBody(httpReq, requestBody, configToRoute)
		if err != nil {
			return nil, nil, err
		}
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
	err := r.cache.Once(&redis.Item{
		Key:   r.cache.GetKeyFromRequestBody(singleRequestBody),
		Value: &cachedResult,
		TTL:   cache.DefaultTTL,
		Do: func(*redis.Item) (interface{}, error) {
			var cacheError error
			respBody, resp, cacheError = r.getResponseBody(httpReq, requestBody, configToRoute)
			defer resp.Body.Close()

			if cacheError != nil {
				return nil, cacheError
			}
			singleResponseBody := respBody.GetSubResponses()[0]
			if singleResponseBody.Error != nil {
				return nil, fmt.Errorf("error found, not caching: %v", singleResponseBody.Error)
			}
			return &singleResponseBody.Result, nil
		},
	})

	// Even if the cache is down, redis-cache will route to the origin
	// properly without error.
	// This conditional handles any unknown errors from redis-cache.
	if err != nil {
		return nil, nil, err
	}

	// Cache hit will mean the response and respBody are empty.
	// Fill in the id and jsonrpc fields in the respBody to match the request.
	if resp == nil && respBody == nil {
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
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := jsonrpc.DecodeResponseBody(resp)

	if err != nil {
		r.logger.Warn("Could not deserialize response.", zap.Any("request", requestBody),
			zap.String("upstreamID", configToRoute.ID), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, err
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
