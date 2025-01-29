package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
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

func (c *RPCCache) HandleRequestParallel(
	chainName string,
	ttl time.Duration,
	reqBody jsonrpc.SingleRequestBody,
	originFunc func() (*jsonrpc.SingleResponseBody, error),
) (json.RawMessage, bool, error) {
	var (
		cached = true
		result json.RawMessage
	)

	ctx := context.Background()

	key := c.CreateRequestKey(chainName, reqBody)
	err := c.Get(ctx, key, &result) // Attempt to fetch from cache
	if err == nil {
		return result, cached, nil // Return if cache hit
	}

	// Cache miss, proceed with the request to origin
	cached = false
	respBody, err := originFunc()
	if err != nil {
		return nil, cached, err
	}

	result = respBody.Result

	// Perform cache set asynchronously
	go func() {
		existingResult := json.RawMessage{}
		if err := c.Get(ctx, key, &existingResult); err == nil {
			if !jsonEqual(existingResult, result) {
				log.Printf("Cache inconsistency detected for key %s", key)
			}
			return // Do not overwrite existing key
		}
		err = c.Set(&cache.Item{Key: c.CreateRequestKey(chainName, reqBody), Value: &result, TTL: ttl})
		if err != nil {
			log.Println("error setting cache", err)
		}
	}()

	return result, cached, nil
}

// jsonEqual checks if two JSON values are equal
func jsonEqual(a, b json.RawMessage) bool {
	var j1, j2 interface{}
	_ = json.Unmarshal(a, &j1)
	_ = json.Unmarshal(b, &j2)
	return reflect.DeepEqual(j1, j2)
}
