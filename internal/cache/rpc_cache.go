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

func (c *RPCCache) HandleRequest(reqBody jsonrpc.SingleRequestBody, originFunc func() (*jsonrpc.SingleResponseBody, error)) (json.RawMessage, error) {
	var cachedResult json.RawMessage

	// Even if the cache is down, redis-cache will route to the origin
	// properly without returning an error.
	err := c.Once(&cache.Item{
		Key:   c.GetKeyFromRequestBody(reqBody),
		Value: &cachedResult,
		TTL:   DefaultTTL,
		Do: func(*cache.Item) (interface{}, error) {
			respBody, err := originFunc()

			// The request to origin fails during cache miss.
			// respBody and resp are nil and nothing should be cached.
			// We should "fail out" of the outer function.
			if err != nil {
				return nil, err
			}
			// On cache miss, the request to origin succeeds but Error is set on responseBody.
			// respBody and resp are not nil and nothing should be cached.
			// respBody and resp should be returned in the outer function.
			singleResponseBody := respBody.GetSubResponses()[0]
			if singleResponseBody.Error != nil {
				return nil, fmt.Errorf("error found, not caching: %v", singleResponseBody.Error)
			}
			return &singleResponseBody.Result, nil
		},
	})

	return cachedResult, err
}
