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

func CreateRedisClient(url string) *redis.ClusterClient {
	if url == "" {
		return nil
	}

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:     []string{url},
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13},
	})

	collector := redisprometheus.NewCollector(metrics.MetricsNamespace, "redis_cache", rdb)
	if err := prometheus.Register(collector); err != nil {
		zap.L().Error("failed to register redis cache otel collector", zap.Error(err))
	}

	return rdb
}

func FromClient(rdb *redis.ClusterClient, metricsContainer *metrics.Container) *RPCCache {
	return &RPCCache{
		cache: cache.New(&cache.Options{
			Redis: rdb,
		}),
		metricsContainer: metricsContainer,
	}
}

type RPCCache struct {
	cache            *cache.Cache
	metricsContainer *metrics.Container
}

func (c *RPCCache) Marshal(value interface{}) ([]byte, error) {
	return c.cache.Marshal(value)
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

	// Counts requests in flight. If it's spiking that means that a lot of requests are for the same key.
	// If there's a too many requests in flight, and redis latency is high it means that the cache is down.
	c.metricsContainer.CacheRequestsInFlight.WithLabelValues(reqBody.Method).Inc()

	start := time.Now()

	var cacheMissDuration, originDuration time.Duration

	// Even if the cache is down, redis-cache will route to the origin
	// properly without returning an error.
	// Do() is executed on cache misses or if the cache is down.
	// Item.Value is still set even on cache miss and the request to origin succeeds.
	// Once() uses request coalescing (single in-flight request to origin even if there are
	// multiple identical incoming requests), which means returned errors
	// could be coming from other goroutines.
	err := c.cache.Once(&cache.Item{
		Key:   c.CreateRequestKey(chainName, reqBody),
		Value: &result,
		TTL:   ttl,
		Do: func(*cache.Item) (interface{}, error) {
			cached = false

			// Capturing the duration from when the request to redis was initiated to when we detect a cache miss.
			cacheMissDuration = time.Since(start) // Time spent on cache lookup
			c.metricsContainer.CacheQueryCacheMissDuration.
				WithLabelValues(reqBody.Method).
				Observe(cacheMissDuration.Seconds())

			originStart := time.Now()
			respBody, err := originFunc()
			originDuration = time.Since(originStart) // Time spent on origin function
			if err != nil {
				return nil, err
			}
			return &respBody.Result, nil
		},
	})

	if err != nil {
		return nil, cached, err
	}

	if cached {
		cacheHitDuration := time.Since(start) // Time spent on cache lookup
		c.metricsContainer.CacheQueryCacheHitDuration.
			WithLabelValues(reqBody.Method).
			Observe(cacheHitDuration.Seconds())
	} else {
		writeDuration := time.Since(start) - cacheMissDuration - originDuration
		c.metricsContainer.CacheWriteDuration.
			WithLabelValues(reqBody.Method).
			Observe(writeDuration.Seconds())
	}

	return result, cached, nil
}
