package cache

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

var methodsToCache = []string{"eth_getTransactionReceipt"}

func NewRPCCache(url string) *RPCCache {
	if url == "" {
		return nil
	}

	// If we start seeing slow cached requests due to network issues,
	// change DialTimeout, ReadTimeout, and WriteTimeout options.
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

func (c *RPCCache) CreateRequestKey(chainName string, requestBody jsonrpc.SingleRequestBody) string {
	return fmt.Sprintf("%s:%s:%v", chainName, requestBody.Method, requestBody.Params)
}

func (c *RPCCache) HandleRequest(chainName string, ttl time.Duration, reqBody jsonrpc.SingleRequestBody, originFunc func() (*jsonrpc.SingleResponseBody, error)) (json.RawMessage, bool, error) {
	var (
		// Technically could be a coalesced request as well.
		cached = true
		result json.RawMessage
	)

	// Even if the cache is down, redis-cache will route to the origin
	// properly without returning an error.
	// Do() is executed on cache misses or if the cache is down.
	// Item.Value is still set even on cache miss and the request to origin succeeds.
	// Once() uses request coalescing (single in-flight request to origin even if there are
	// multiple identical incoming requests), which means returned errors
	// could be coming from other goroutines.
	err := c.Once(&cache.Item{
		Key:   c.CreateRequestKey(chainName, reqBody),
		Value: &result,
		TTL:   ttl,
		Do: func(*cache.Item) (interface{}, error) {
			cached = false
			respBody, err := originFunc()
			if err != nil {
				return nil, err
			}
			return &respBody.Result, nil
		},
	})

	if err != nil {
		return nil, cached, err
	}

	return result, cached, nil
}
