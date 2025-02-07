package cache

import (
	"testing"

	"github.com/go-redis/redismock/v9"
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
