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
	"github.com/satsuma-data/node-gateway/internal/util"
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
	err          error
	response     string
	ResponseCode int
}

type HTTPResponse struct {
	StatusCode int
}

func (e *OriginError) Error() string {
	return fmt.Sprintf("error making request to origin. err: %v, resp: %s, respCode: %d", e.err, e.response, e.ResponseCode)
}

/*
 * Return arguments
 * 1. JSON Response body - Body of response decoded into JSON RPC, if possible.
 * 2. HTTP Response - HTTP response from upstream, if possible.
 * 3. Cached - Whether the response was cached.
 * 4. Error - Error encountered when making request to upstream or another error encountered during processing.
 */
func (r *RequestExecutor) routeToConfig(
	ctx context.Context,
	requestBody jsonrpc.RequestBody,
	configToRoute *config.UpstreamConfig,
) (jsonrpc.ResponseBody, *HTTPResponse, bool, error) {
	bodyBytes, err := requestBody.Encode()
	if err != nil {
		r.logger.Error("Could not serialize request.", zap.Any("request", requestBody), zap.Error(err))
		return nil, nil, false, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	if err != nil {
		r.logger.Error("Could not create new http request.", zap.Any("request", requestBody),
			zap.String("upstreamID", configToRoute.ID), zap.Error(err))
		return nil, nil, false, err
	}

	httpReq.Header.Set("content-type", "application/json")

	if configToRoute.RequestHeadersConfig != nil {
		for _, headerConfig := range configToRoute.RequestHeadersConfig {
			httpReq.Header.Set(headerConfig.Key, headerConfig.Value)
		}
	}

	if configToRoute.BasicAuthConfig.Username != "" && configToRoute.BasicAuthConfig.Password != "" {
		encodedCredentials := base64.StdEncoding.EncodeToString([]byte(configToRoute.BasicAuthConfig.Username + ":" + configToRoute.BasicAuthConfig.Password))
		httpReq.Header.Set("Authorization", "Basic "+encodedCredentials)
	}

	var (
		jsonRespBody jsonrpc.ResponseBody
		httpResp     *HTTPResponse
	)

	singleRequestBody, isSingleRequestBody := requestBody.(*jsonrpc.SingleRequestBody)

	if r.useCache(requestBody) && isSingleRequestBody {
		var cached bool
		// In case of unknown caching errors, the httpReq might get used twice.
		// We must clone the httpReq otherwise the body will already be closed on the second request.
		jsonRespBody, httpResp, cached, err = r.retrieveOrCacheRequest(cloneRequest(httpReq), *singleRequestBody, configToRoute)
		if err != nil {
			switch e := err.(type) {
			case *OriginError, *jsonrpc.DecodeError:
				// These errors indicates a cache miss and request failure to origin.
				// We want this error to bubble up.
				// Unknown cache errors will default back to the "non-caching" behavior.
				r.logger.Warn("caching error making request to origin", zap.Error(err), zap.Any("request", requestBody), zap.Any("resp", httpResp))
				return nil, httpResp, cached, e
			default:
				r.logger.Warn("unknown caching error", zap.Error(err), zap.Any("request", requestBody), zap.Any("resp", httpResp))
			}
		} else {
			return jsonRespBody, httpResp, cached, nil
		}
	}

	jsonRespBody, httpResp, err = r.getResponseBody(httpReq, requestBody, configToRoute)

	return jsonRespBody, httpResp, false, err
}

func (r *RequestExecutor) useCache(requestBody jsonrpc.RequestBody) bool {
	if r.cache == nil || r.cacheConfig.TTL == 0 {
		return false
	}

	return r.cache.ShouldCacheMethod(requestBody.GetMethod())
}

func (r *RequestExecutor) retrieveOrCacheRequest(httpReq *http.Request, requestBody jsonrpc.SingleRequestBody, configToRoute *config.UpstreamConfig) (jsonrpc.ResponseBody, *HTTPResponse, bool, error) {
	var (
		jsonRPCRespBody jsonrpc.ResponseBody
		httpResp        *HTTPResponse
	)

	originFunc := func() (*jsonrpc.SingleResponseBody, error) {
		var err error

		// Any errors will result in respBody and resp being nil.
		jsonRPCRespBody, httpResp, err = r.getResponseBody(httpReq, &requestBody, configToRoute)
		if err != nil {
			return nil, err
		}

		singleRespBody, ok := jsonRPCRespBody.(*jsonrpc.SingleResponseBody)
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

	val, cached, err := r.cache.HandleRequestParallel(r.chainName, requestBody, originFunc)

	if err != nil {
		switch err := err.(type) {
		case *HandledError:
			// The cache uses request coalescing, an error may be returned by another goroutine.
			// Construct a responsebody and a fake response.
			if httpResp == nil && jsonRPCRespBody == nil {
				httpResp = &HTTPResponse{
					StatusCode: http.StatusOK,
				}
				rb := *err.rb
				rb.ID = *requestBody.ID
				rb.JSONRPC = requestBody.JSONRPCVersion
				jsonRPCRespBody = &rb
			}
		default:
			return nil, httpResp, cached, err
		}
	}

	if httpResp == nil && jsonRPCRespBody == nil {
		if val == nil {
			return nil, nil, cached, fmt.Errorf("unexpected empty response from cache")
		}

		r.logger.Debug("cache hit", zap.Any("request", requestBody), zap.Any("value", val))

		// Fill in id and jsonrpc in the respBody to match the request.
		httpResp = &HTTPResponse{
			StatusCode: http.StatusOK,
		}
		jsonRPCRespBody = &jsonrpc.SingleResponseBody{
			ID:      *requestBody.ID,
			JSONRPC: requestBody.JSONRPCVersion,
			Result:  val,
		}
	}

	return jsonRPCRespBody, httpResp, cached, nil
}

func (r *RequestExecutor) getResponseBody(httpReq *http.Request, requestBody jsonrpc.RequestBody, configToRoute *config.UpstreamConfig) (jsonrpc.ResponseBody, *HTTPResponse, error) {
	resp, err := r.httpClient.Do(httpReq)

	if err != nil {
		r.logger.Error("Error encountered when executing request.", zap.Any("request", requestBody),
			zap.String("upstreamID", configToRoute.ID), zap.Error(err))

		return nil, nil, &OriginError{err, "", 0}
	}

	httpResp := &HTTPResponse{resp.StatusCode}

	// Body can only be read once. Read it out and put it back in the response.
	respBodyBytes, err := util.ReadAndCopyBackResponseBody(resp)
	respBodyString := string(respBodyBytes)

	if err != nil {
		r.logger.Error("Error encountered when reading response body.", zap.Any("request", requestBody),
			zap.String("response", respBodyString), zap.String("upstreamID", configToRoute.ID), zap.Error(err))

		return nil, httpResp, &OriginError{err, "", 0}
	}

	if resp.StatusCode >= http.StatusBadRequest {
		r.logger.Error("4xx/5xx status code encountered when executing request.", zap.Any("request", requestBody),
			zap.String("upstreamID", configToRoute.ID), zap.String("response", respBodyString),
			zap.Int("httpStatusCode", resp.StatusCode), zap.Error(err))

		return nil, httpResp, &OriginError{nil, respBodyString, resp.StatusCode}
	}

	defer resp.Body.Close()

	jsonRPCBody, err := jsonrpc.DecodeResponseBody(respBodyBytes)

	if err != nil {
		r.logger.Warn("Could not deserialize response.", zap.Any("request", requestBody),
			zap.String("upstreamID", configToRoute.ID), zap.String("response", respBodyString), zap.Error(err))
		return nil, httpResp, err
	}

	r.logger.Debug("Successfully routed request to upstream.", zap.String("upstreamID", configToRoute.ID), zap.Any("request", requestBody), zap.Any("response", jsonRPCBody))

	return jsonRPCBody, httpResp, nil
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
