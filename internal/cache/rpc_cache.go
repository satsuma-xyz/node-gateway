package cache

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/cache/v9"
	"github.com/prometheus/client_golang/prometheus"
	redisprometheus "github.com/redis/go-redis/extra/redisprometheus/v9"
	"github.com/redis/go-redis/v9"
	"github.com/samber/lo"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"go.uber.org/zap"
)

var methodsToCache = []string{"eth_getTransactionReceipt"}

func NewRPCCache(url string) *RPCCache {
	if url == "" {
		return nil
	}

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:     []string{url},
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13},
	})

	collector := redisprometheus.NewCollector(metrics.MetricsNamespace, "redis_cache", rdb)
	if err := prometheus.Register(collector); err != nil {
		zap.L().Error("failed to register redis cache collector", zap.Error(err))
	}

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
