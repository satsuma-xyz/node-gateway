package route

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	redis "github.com/go-redis/cache/v9"
	"github.com/go-redis/redismock/v9"
	"github.com/samber/lo"
	"github.com/satsuma-data/node-gateway/internal/cache"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestRetrieveOrCacheRequest(t *testing.T) {
	redisClient, redisClientMock := redismock.NewClientMock()
	rpcCache := &cache.RPCCache{
		Cache: redis.New(&redis.Options{
			Redis: redisClient,
		}),
	}
	httpResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"id":1,"jsonrpc":"2.0","result":"hello"}`)),
	}

	cacheConfig := config.ChainCacheConfig{
		TTL: 6 * time.Second,
	}

	httpClientMock := mocks.NewHTTPClient(t)
	// We only expect the mock to be called once.
	// The second call to retrieveOrCacheRequest should be cached.
	httpClientMock.On("Do", mock.Anything).Return(httpResp, nil).Once()
	executor := RequestExecutor{httpClientMock, zap.L(), rpcCache, "mainnet", cacheConfig}

	ctx := context.Background()
	requestBody := jsonrpc.SingleRequestBody{
		ID:             lo.ToPtr[int64](1),
		JSONRPCVersion: "2.0",
		Method:         "eth_getTransactionReceipt",
		Params:         []any{"0xa8b4537fa06ea76df9498fc50cd59fc298e5f5e4c708dc3c82fd021fc230869d"},
	}

	configToRoute := config.UpstreamConfig{
		ID:      "geth",
		GroupID: "primary",
		HTTPURL: "gethURL",
	}

	bodyBytes, _ := requestBody.Encode()

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))

	cacheKey := rpcCache.CreateRequestKey("mainnet", requestBody)
	redisClientMock.ExpectGet(cacheKey).SetErr(errors.New("error"))
	// The cache has custom marshaling to pack the cache efficiently.
	raw := json.RawMessage(`"hello"`)
	expected, _ := rpcCache.Marshal(raw)
	redisClientMock.ExpectSet(cacheKey, expected, cacheConfig.TTL).SetVal("OK")

	jsonRPCResponseBody, httpResponse, cached, _ := executor.retrieveOrCacheRequest(httpReq, requestBody, &configToRoute)

	singleRespBody := jsonRPCResponseBody.GetSubResponses()[0]

	assert.Equal(t, int64(1), singleRespBody.ID)
	assert.Equal(t, "2.0", singleRespBody.JSONRPC)
	assert.Nil(t, singleRespBody.Error)
	assert.Equal(t, raw, singleRespBody.Result)
	assert.Equal(t, httpResp.StatusCode, httpResponse.StatusCode)
	assert.False(t, cached)

	// Send a new request with new ID.
	// The results should be cached.
	requestBody.ID = lo.ToPtr[int64](20)
	bodyBytes, _ = requestBody.Encode()
	httpReq, _ = http.NewRequestWithContext(ctx, "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))

	// SetVal simulates returned value on a Get cache hit.
	redisClientMock.ExpectGet(cacheKey).SetVal(bytes.NewBuffer(expected).String())

	jsonRPCResponseBody, httpResponse, cached, _ = executor.retrieveOrCacheRequest(httpReq, requestBody, &configToRoute)

	singleRespBody = jsonRPCResponseBody.GetSubResponses()[0]

	assert.Equal(t, int64(20), singleRespBody.ID)
	assert.Equal(t, "2.0", singleRespBody.JSONRPC)
	assert.Nil(t, singleRespBody.Error)
	assert.Equal(t, raw, singleRespBody.Result)
	assert.Equal(t, httpResp.StatusCode, httpResponse.StatusCode)
	assert.True(t, cached)

	if err := redisClientMock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestRetrieveOrCacheRequest_OriginError(t *testing.T) {
	redisClient, _ := redismock.NewClientMock()
	rpcCache := &cache.RPCCache{
		Cache: redis.New(&redis.Options{
			Redis: redisClient,
		}),
	}
	httpResp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader("error")),
	}

	cacheConfig := config.ChainCacheConfig{
		TTL: 6 * time.Second,
	}

	httpClientMock := mocks.NewHTTPClient(t)
	httpClientMock.On("Do", mock.Anything).Return(httpResp, nil).Once()
	executor := RequestExecutor{httpClientMock, zap.L(), rpcCache, "mainnet", cacheConfig}

	ctx := context.Background()
	requestBody := jsonrpc.SingleRequestBody{
		ID:             lo.ToPtr[int64](1),
		JSONRPCVersion: "2.0",
		Method:         "eth_getTransactionReceipt",
		Params:         []any{"0xa8b4537fa06ea76df9498fc50cd59fc298e5f5e4c708dc3c82fd021fc230869d"},
	}

	configToRoute := config.UpstreamConfig{
		ID:      "geth",
		GroupID: "primary",
		HTTPURL: "gethURL",
	}

	bodyBytes, _ := requestBody.Encode()
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))

	respBody, resp, _, err := executor.retrieveOrCacheRequest(httpReq, requestBody, &configToRoute)

	assert.Nil(t, respBody)
	assert.Equal(t, 500, resp.StatusCode)

	_, ok := err.(*OriginError)
	assert.True(t, ok)
}

func TestRetrieveOrCacheRequest_JSONRPCError(t *testing.T) {
	redisClient, _ := redismock.NewClientMock()
	rpcCache := &cache.RPCCache{
		Cache: redis.New(&redis.Options{
			Redis: redisClient,
		}),
	}
	httpResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"id":1,"jsonrpc":"2.0","error":{"code":1,"message":"RPC error"}}`)),
	}
	cacheConfig := config.ChainCacheConfig{
		TTL: 6 * time.Second,
	}

	httpClientMock := mocks.NewHTTPClient(t)
	httpClientMock.On("Do", mock.Anything).Return(httpResp, nil).Once()
	executor := RequestExecutor{httpClientMock, zap.L(), rpcCache, "mainnet", cacheConfig}

	ctx := context.Background()
	requestBody := jsonrpc.SingleRequestBody{
		ID:             lo.ToPtr[int64](1),
		JSONRPCVersion: "2.0",
		Method:         "eth_getTransactionReceipt",
		Params:         []any{"0xa8b4537fa06ea76df9498fc50cd59fc298e5f5e4c708dc3c82fd021fc230869d"},
	}

	configToRoute := config.UpstreamConfig{
		ID:      "geth",
		GroupID: "primary",
		HTTPURL: "gethURL",
	}

	bodyBytes, _ := requestBody.Encode()
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	respBody, resp, _, err := executor.retrieveOrCacheRequest(httpReq, requestBody, &configToRoute)

	singleRespBody := respBody.GetSubResponses()[0]

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, int64(1), singleRespBody.ID)
	assert.Equal(t, "2.0", singleRespBody.JSONRPC)
	assert.Equal(t, "RPC error", singleRespBody.Error.Message)
	assert.Nil(t, singleRespBody.Result)
}

func TestRetrieveOrCacheRequest_NullResultError(t *testing.T) {
	redisClient, _ := redismock.NewClientMock()
	rpcCache := &cache.RPCCache{
		Cache: redis.New(&redis.Options{
			Redis: redisClient,
		}),
	}
	httpResp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`{"id":1,"jsonrpc":"2.0","result":null}`)),
	}
	cacheConfig := config.ChainCacheConfig{
		TTL: 6 * time.Second,
	}

	httpClientMock := mocks.NewHTTPClient(t)
	httpClientMock.On("Do", mock.Anything).Return(httpResp, nil).Once()
	executor := RequestExecutor{httpClientMock, zap.L(), rpcCache, "mainnet", cacheConfig}

	ctx := context.Background()
	requestBody := jsonrpc.SingleRequestBody{
		ID:             lo.ToPtr[int64](1),
		JSONRPCVersion: "2.0",
		Method:         "eth_getTransactionReceipt",
		Params:         []any{"0xa8b4537fa06ea76df9498fc50cd59fc298e5f5e4c708dc3c82fd021fc230869d"},
	}

	configToRoute := config.UpstreamConfig{
		ID:      "geth",
		GroupID: "primary",
		HTTPURL: "gethURL",
	}

	bodyBytes, _ := requestBody.Encode()
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	respBody, resp, _, err := executor.retrieveOrCacheRequest(httpReq, requestBody, &configToRoute)

	singleRespBody := respBody.GetSubResponses()[0]

	assert.Nil(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, int64(1), singleRespBody.ID)
	assert.Equal(t, "2.0", singleRespBody.JSONRPC)
	assert.Equal(t, json.RawMessage("null"), singleRespBody.Result)
}

func TestUseCache(t *testing.T) {
	redisClient, _ := redismock.NewClientMock()
	rpcCache := &cache.RPCCache{
		Cache: redis.New(&redis.Options{
			Redis: redisClient,
		}),
	}

	requestBody := &jsonrpc.SingleRequestBody{
		ID:             lo.ToPtr[int64](1),
		JSONRPCVersion: "2.0",
		Method:         "eth_getTransactionReceipt",
		Params:         []any{"0xa8b4537fa06ea76df9498fc50cd59fc298e5f5e4c708dc3c82fd021fc230869d"},
	}

	cacheConfig := config.ChainCacheConfig{
		TTL: 6 * time.Second,
	}

	var tests = []struct {
		requestBody jsonrpc.RequestBody
		cache       *cache.RPCCache
		name        string
		cacheConfig config.ChainCacheConfig
		want        bool
	}{
		{
			name:        "normal case",
			cache:       rpcCache,
			cacheConfig: cacheConfig,
			requestBody: requestBody,
			want:        true,
		},
		{
			name:        "cache is nil",
			cache:       nil,
			cacheConfig: cacheConfig,
			requestBody: requestBody,
			want:        false,
		},
		{
			name:  "cache TTL is zero",
			cache: rpcCache,
			cacheConfig: config.ChainCacheConfig{
				TTL: 0,
			},
			requestBody: requestBody,
			want:        false,
		},
		{
			name:        "JSON RPC method does not match",
			cache:       rpcCache,
			cacheConfig: cacheConfig,
			requestBody: &jsonrpc.SingleRequestBody{
				ID:             lo.ToPtr[int64](1),
				JSONRPCVersion: "2.0",
				Method:         "eth_someFakeCall",
				Params:         []any{"0xa8b4537fa06ea76df9498fc50cd59fc298e5f5e4c708dc3c82fd021fc230869d"},
			},
			want: false,
		},
	}

	httpClientMock := mocks.NewHTTPClient(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := RequestExecutor{httpClientMock, zap.L(), tt.cache, "mainnet", tt.cacheConfig}
			ans := executor.useCache(tt.requestBody)
			if ans != tt.want { //nolint:nolintlint,wsl // Legacy
				t.Errorf("got %t, want %t", ans, tt.want)
			}
		})
	}
}
