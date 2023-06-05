package cache

import (
	"fmt"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

var methodsToCache = []string{"eth_getTransactionReceipt"}

const DefaultTTL = 30 * time.Minute

func NewRPCCache(url string) *RPCCache {
	if url == "" {
		return nil
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: url,
	})

	return &RPCCache{
		cache.New(&cache.Options{
			Redis: rdb,
		}),
	}
}

type RPCCache struct {
	*cache.Cache
}

func (c *RPCCache) ShouldCacheMethod(method string) bool {
	return lo.Contains(methodsToCache, method)
}

func (c *RPCCache) GetKeyFromRequestBody(requestBody jsonrpc.SingleRequestBody) string {
	return fmt.Sprintf("%s:%v", requestBody.Method, requestBody.Params)
}
