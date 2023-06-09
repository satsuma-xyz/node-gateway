package cache

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

var methodsToCache = []string{"eth_getTransactionReceipt"}

const DefaultTTL = 1 * time.Second

type JSONRPCError struct {
	err *jsonrpc.Error
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("error found in JSON RPC response: %v", e.err)
}

type NullResultError struct {
}

func (e *NullResultError) Error() string {
	return "JSON RPC response has null Result field."
}

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

func (c *RPCCache) HandleRequest(chainName string, ttl time.Duration, reqBody jsonrpc.SingleRequestBody, originFunc func() (*jsonrpc.SingleResponseBody, error)) (json.RawMessage, error) {
	var result json.RawMessage

	// Even if the cache is down, redis-cache will route to the origin
	// properly without returning an error.
	// Do() is executed on cache misses or if the cache is down.
	// Item.Value is still set even on cache miss and the request to origin succeeds.
	err := c.Once(&cache.Item{
		Key:   c.CreateRequestKey(chainName, reqBody),
		Value: &result,
		TTL:   ttl,
		Do: func(*cache.Item) (interface{}, error) {
			respBody, err := originFunc()
			if err != nil {
				return nil, err
			}
			// Check that the Error field is set on responseBody.
			// Even though this is a successful HTTP request, we do not want to cache JSONRPC errors.
			singleResponseBody := respBody.GetSubResponses()[0]
			if singleResponseBody.Error != nil {
				return nil, &JSONRPCError{singleResponseBody.Error}
			}
			r := bytes.NewBuffer(singleResponseBody.Result).String()
			if r == "null" {
				return nil, &NullResultError{}
			}
			return &singleResponseBody.Result, nil
		},
	})

	if err != nil {
		switch err.(type) {
		// A JSON RPC error should be returned to the user but not cached.
		case *JSONRPCError:
			return nil, nil
		// A null Result field response should be returned to the user but not cached.
		case *NullResultError:
			return nil, nil
		}

		return nil, err
	}

	return result, nil
}
