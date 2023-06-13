package route

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"net/http"

	"github.com/satsuma-data/node-gateway/internal/cache"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"go.uber.org/zap"
)

type RequestExecutor struct {
	httpClient  client.HTTPClient
	logger      *zap.Logger
	cache       *cache.RPCCache
	chainName   string
	cacheConfig config.ChainCacheConfig
}

type HandledError struct {
	rb *jsonrpc.SingleResponseBody
}

func (e *HandledError) Error() string {
	return fmt.Sprintf("bubbling error response back to user: %v", e.rb)
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

	singleRequestBody, isSingleRequestBody := requestBody.(*jsonrpc.SingleRequestBody)

	if r.useCache(requestBody) && isSingleRequestBody {
		// In case of unknown caching errors, the httpReq might get used twice.
		// We must clone the httpReq otherwise the body will already be closed on the second request.
		respBody, resp, err = r.retrieveOrCacheRequest(cloneRequest(httpReq), *singleRequestBody, configToRoute)
		if err != nil {
			originError, _ := err.(*OriginError)
			// An OriginError indicates a cache miss and request failure to origin.
			// We want this error to bubble up.
			// Unknown cache errors will default back to the "non-caching" behavior.
			if originError != nil {
				r.logger.Warn("caching error making request to origin", zap.Error(err), zap.Any("request", requestBody))
				return nil, nil, originError
			}

			r.logger.Warn("unknown caching error", zap.Error(err), zap.Any("request", requestBody))
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

func (r *RequestExecutor) useCache(requestBody jsonrpc.RequestBody) bool {
	if r.cache == nil || r.cacheConfig.TTL == 0 {
		return false
	}

	return r.cache.ShouldCacheMethod(requestBody.GetMethod())
}

func (r *RequestExecutor) retrieveOrCacheRequest(httpReq *http.Request, requestBody jsonrpc.SingleRequestBody, configToRoute *config.UpstreamConfig) (jsonrpc.ResponseBody, *http.Response, error) {
	var (
		respBody jsonrpc.ResponseBody
		resp     *http.Response
	)

	originFunc := func() (*jsonrpc.SingleResponseBody, error) {
		var err error

		// Any errors will result in respBody and resp being nil.
		respBody, resp, err = r.getResponseBody(httpReq, &requestBody, configToRoute) //nolint:bodyclose // linter bug
		if err != nil {
			return nil, err
		}

		singleRespBody, ok := respBody.(*jsonrpc.SingleResponseBody)
		if !ok {
			return nil, errors.New("batched responses do not support caching")
		}

		if singleRespBody.Error != nil {
			r.logger.Debug("JSON RPC response has Error field set", zap.Any("request", requestBody), zap.Any("respBody", singleRespBody))
			return nil, &HandledError{singleRespBody}
		}

		result := bytes.NewBuffer(singleRespBody.Result).String()
		if result == "null" {
			r.logger.Debug("null result", zap.Any("request", requestBody), zap.Any("respBody", singleRespBody))
			return nil, &HandledError{singleRespBody}
		}

		return singleRespBody, nil
	}

	val, err := r.cache.HandleRequest(r.chainName, r.cacheConfig.TTL, requestBody, originFunc)

	if err != nil {
		switch err := err.(type) {
		case *HandledError:
			// The cache uses request coalescing, an error may be returned by another goroutine.
			// Construct a responsebody and a fake response.
			if resp == nil && respBody == nil {
				resp = &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(new(bytes.Buffer)),
				}
				rb := *err.rb
				rb.ID = *requestBody.ID
				rb.JSONRPC = requestBody.JSONRPCVersion
				respBody = &rb
			}
		default:
			return nil, nil, err
		}
	}

	if resp == nil && respBody == nil {
		if val == nil {
			return nil, nil, fmt.Errorf("unexpected empty response from cache")
		}

		r.logger.Debug("cache hit", zap.Any("request", requestBody), zap.Any("value", val))

		// Fill in id and jsonrpc in the respBody to match the request.
		resp = &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(new(bytes.Buffer)),
		}
		respBody = &jsonrpc.SingleResponseBody{
			ID:      *requestBody.ID,
			JSONRPC: requestBody.JSONRPCVersion,
			Result:  val,
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
