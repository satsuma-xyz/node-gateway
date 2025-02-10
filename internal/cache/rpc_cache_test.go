package cache

import (
	"bytes"
	"encoding/json"
	"errors"

	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/redis/go-redis/v9"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/stretchr/testify/assert"
)

func TestShouldCacheMethod(t *testing.T) {
	redisClient, _ := redismock.NewClientMock()
	cache := FromClients(redisClient, redisClient, metrics.NewContainer(config.TestChainName))
	assert.True(t, cache.ShouldCacheMethod("eth_getTransactionReceipt"))

	assert.False(t, cache.ShouldCacheMethod("eth_getBlockByHash"))
	assert.False(t, cache.ShouldCacheMethod("eth_getBlockByNumber"))
	assert.False(t, cache.ShouldCacheMethod("eth_getLogs"))
}

func TestCreateRequestKey(t *testing.T) {
	singleRequestBody := jsonrpc.SingleRequestBody{
		Method: "eth_getTransactionReceipt",
		Params: []any{"0x3a6f67beb73d07b1dd10c12de79767b6009f7b351ba1fe6282040aa6c57afef1"},
	}
	assert.Equal(t, "mainnet:eth_getTransactionReceipt:[0x3a6f67beb73d07b1dd10c12de79767b6009f7b351ba1fe6282040aa6c57afef1]", CreateRequestKey("mainnet", singleRequestBody))
}

func TestHandleRequestParallel(t *testing.T) {
	redisReadClient, redisReadClientMock := redismock.NewClientMock()
	redisWriteClient, redisWriteClientMock := redismock.NewClientMock()
	metricsContainer := metrics.NewContainer(config.TestChainName)
	cache := FromClients(redisReadClient, redisWriteClient, metricsContainer)

	chainName := "mainnet"
	ttl := 5 * time.Minute
	reqBody := jsonrpc.SingleRequestBody{
		Method: "eth_getTransactionReceipt",
		Params: []any{"0x123"},
	}
	cacheKey := CreateRequestKey(chainName, reqBody)
	expectedResult := json.RawMessage(`{"test":"value"}`)
	expectedResultBytes, _ := cache.Marshal(expectedResult)

	tests := []struct {
		mockSetup      func()
		after          func()
		originResponse *jsonrpc.SingleResponseBody
		originError    error
		name           string
		description    string
		wantCached     bool
		wantError      bool
	}{
		{
			name: "cache_hit",
			mockSetup: func() {
				redisReadClientMock.ExpectGet(cacheKey).SetVal(bytes.NewBuffer(expectedResultBytes).String())
			},
			after: func() {
				cache.cacheRead.DeleteFromLocalCache(cacheKey)
				cache.cacheWrite.DeleteFromLocalCache(cacheKey)
			},
			wantCached:  true,
			wantError:   false,
			description: "Should return cached value on cache hit",
		},
		{
			name: "cache_miss_success",
			mockSetup: func() {
				redisReadClientMock.ExpectGet(cacheKey).SetErr(redis.Nil)
				redisWriteClientMock.ExpectSetNX(cacheKey, expectedResultBytes, ttl).SetVal(true)
			},
			after: func() {
				cache.cacheRead.DeleteFromLocalCache(cacheKey)
				cache.cacheWrite.DeleteFromLocalCache(cacheKey)
			},
			originResponse: &jsonrpc.SingleResponseBody{
				Result: expectedResult,
			},
			wantCached:  false,
			wantError:   false,
			description: "Should fetch from origin on cache miss",
		},
		{
			name: "cache_miss_origin_error",
			mockSetup: func() {
				redisReadClientMock.ExpectGet(cacheKey).SetErr(redis.Nil)
			},
			after:       func() {},
			originError: errors.New("origin error"),
			wantCached:  false,
			wantError:   true,
			description: "Should return error when origin fails",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			if tt.after != nil {
				defer tt.after()
			}

			originFunc := func() (*jsonrpc.SingleResponseBody, error) {
				if tt.originError != nil {
					return nil, tt.originError
				}

				return tt.originResponse, nil
			}

			result, cached, err := cache.HandleRequestParallel(chainName, ttl, reqBody, originFunc)

			// Add small sleep to allow async cache set to complete
			time.Sleep(5 * time.Millisecond)

			// Verify error handling
			if (err != nil) != tt.wantError {
				t.Errorf("HandleRequestParallel() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// Verify cache status
			if cached != tt.wantCached {
				t.Errorf("HandleRequestParallel() cached = %v, want %v", cached, tt.wantCached)
			}

			// On successful cache hit or origin response, verify result
			if !tt.wantError {
				if !bytes.Equal(result, expectedResult) {
					t.Errorf("HandleRequestParallel() result = %v, want %v", result, expectedResult)
				}
			}

			// Verify all Redis expectations were met
			if err := redisReadClientMock.ExpectationsWereMet(); err != nil {
				t.Errorf("Redis expectations not met: %v", err)
			}

			if err := redisWriteClientMock.ExpectationsWereMet(); err != nil {
				t.Errorf("Redis expectations not met: %v", err)
			}
		})
	}
}
