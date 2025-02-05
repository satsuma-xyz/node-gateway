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
	redisClient, _ := redismock.NewClusterMock()
	cache := FromClient(redisClient, metrics.NewContainer(config.TestChainName))
	assert.True(t, cache.ShouldCacheMethod("eth_getTransactionReceipt"))

	assert.False(t, cache.ShouldCacheMethod("eth_getBlockByHash"))
	assert.False(t, cache.ShouldCacheMethod("eth_getBlockByNumber"))
	assert.False(t, cache.ShouldCacheMethod("eth_getLogs"))
}

func TestCreateRequestKey(t *testing.T) {
	redisClient, _ := redismock.NewClusterMock()
	cache := FromClient(redisClient, metrics.NewContainer(config.TestChainName))
	singleRequestBody := jsonrpc.SingleRequestBody{
		Method: "eth_getTransactionReceipt",
		Params: []any{"0x3a6f67beb73d07b1dd10c12de79767b6009f7b351ba1fe6282040aa6c57afef1"},
	}
	assert.Equal(t, "mainnet:eth_getTransactionReceipt:[0x3a6f67beb73d07b1dd10c12de79767b6009f7b351ba1fe6282040aa6c57afef1]", cache.CreateRequestKey("mainnet", singleRequestBody))
}
