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
	redisClient, _ := redismock.NewClientMock()
	cache := FromClients(redisClient, redisClient, metrics.NewContainer(config.TestChainName))
	singleRequestBody := jsonrpc.SingleRequestBody{
		Method: "eth_getTransactionReceipt",
		Params: []any{"0x3a6f67beb73d07b1dd10c12de79767b6009f7b351ba1fe6282040aa6c57afef1"},
	}
	assert.Equal(t, "mainnet:eth_getTransactionReceipt:[0x3a6f67beb73d07b1dd10c12de79767b6009f7b351ba1fe6282040aa6c57afef1]", cache.CreateRequestKey("mainnet", singleRequestBody))
}

func TestGetRedisAddresses(t *testing.T) {
	tests := []struct {
		name        string
		config      config.CacheConfig
		wantReader  string
		wantWriter  string
		description string
	}{
		{
			name: "legacy_config",
			config: config.CacheConfig{
				Redis: "localhost:6379",
			},
			wantReader:  "localhost:6379",
			wantWriter:  "localhost:6379",
			description: "Should use Redis field for both when only legacy config provided",
		},
		{
			name: "split_config",
			config: config.CacheConfig{
				RedisReader: "reader:6379",
				RedisWriter: "writer:6379",
			},
			wantReader:  "reader:6379",
			wantWriter:  "writer:6379",
			description: "Should use separate addresses when both reader and writer specified",
		},
		{
			name: "mixed_config",
			config: config.CacheConfig{
				Redis:       "legacy:6379",
				RedisReader: "reader:6379",
				RedisWriter: "writer:6379",
			},
			wantReader:  "reader:6379",
			wantWriter:  "writer:6379",
			description: "Should prefer new config over legacy when both present",
		},
		{
			name: "partial_config",
			config: config.CacheConfig{
				Redis:       "legacy:6379",
				RedisReader: "reader:6379",
			},
			wantReader:  "legacy:6379",
			wantWriter:  "legacy:6379",
			description: "Should fall back to legacy when new config is incomplete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, writer := GetRedisAddresses(tt.config)
			assert.Equal(t, tt.wantReader, reader, tt.description)
			assert.Equal(t, tt.wantWriter, writer, tt.description)
		})
	}
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
	cacheKey := cache.CreateRequestKey(chainName, reqBody)
	expectedResult := json.RawMessage(`{"test":"value"}`)

	tests := []struct {
		mockSetup      func()
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
				redisReadClientMock.ExpectGet(cacheKey).SetVal(string(expectedResult))
			},
			wantCached:  true,
			wantError:   false,
			description: "Should return cached value on cache hit",
		},
		{
			name: "cache_miss_success",
			mockSetup: func() {
				redisReadClientMock.ExpectGet(cacheKey).SetErr(redis.Nil)
				redisWriteClientMock.ExpectSet(cacheKey, &expectedResult, ttl).SetVal("OK")
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
			originError: errors.New("origin error"),
			wantCached:  false,
			wantError:   true,
			description: "Should return error when origin fails",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

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
