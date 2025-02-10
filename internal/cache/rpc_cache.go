package cache

import (
	"context"
	"encoding/json"
	"fmt"

	"strconv"
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

var redisDialTimeout = 2 * time.Second
var redisReadTimeout = 500 * time.Millisecond
var redisWriteTimeout = 500 * time.Millisecond

// var redisPoolTimeout = 100 * time.Millisecond
// var redisPoolSize = runtime.NumCPU() * 30

func CreateRedisClient(url string) *redis.Client {
	if url == "" {
		return nil
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         url,
		DialTimeout:  redisDialTimeout,
		ReadTimeout:  redisReadTimeout,
		WriteTimeout: redisWriteTimeout,
		// PoolTimeout:  redisPoolTimeout,
		// PoolSize:     redisPoolSize,
		OnConnect: func(_ context.Context, _ *redis.Conn) error {
			zap.L().Info("established new connection to redis", zap.String("url", url))
			metrics.CacheConnections.WithLabelValues(url).Inc()
			return nil
		},
	})

	collector := redisprometheus.NewCollector(metrics.MetricsNamespace, "redis_cache", rdb)
	if err := prometheus.Register(collector); err != nil {
		zap.L().Error("failed to register redis cache otel collector", zap.Error(err))
	}

	return rdb
}

func FromClients(reader, writer *redis.Client, metricsContainer *metrics.Container) *RPCCache {
	if reader == nil || writer == nil {
		return nil
	}

	return &RPCCache{
		cache: cache.New(&cache.Options{
			Redis: writer,
		}),
		readClient:       reader,
		writeClient:      writer,
		metricsContainer: metricsContainer,
	}
}

func FromClient(rdb *redis.Client, metricsContainer *metrics.Container) *RPCCache {
	return FromClients(rdb, rdb, metricsContainer)
}

type RPCCache struct {
	cache            *cache.Cache
	readClient       *redis.Client
	writeClient      *redis.Client
	metricsContainer *metrics.Container
}

func (c *RPCCache) get(ctx context.Context, key, jsonRPCMethod string) (json.RawMessage, error) {
	start := time.Now()
	cmd := c.readClient.Get(ctx, key)
	duration := time.Since(start)

	result, err := cmd.Result()
	cacheMiss := err == redis.Nil

	// Record metrics
	c.metricsContainer.CacheReadDuration.
		WithLabelValues(
			jsonRPCMethod,
			strconv.FormatBool(!cacheMiss)).
		Observe(duration.Seconds())

	zap.L().Debug("cache_get",
		zap.String("jsonRPCMethod", jsonRPCMethod),
		zap.Bool("cacheHit", !cacheMiss),
		zap.String("key", key),
		zap.Int64("durationMs", duration.Milliseconds()))

	if cacheMiss {
		return nil, err
	}

	if err != nil {
		c.metricsContainer.CacheErrors.WithLabelValues("get").Inc()
		zap.L().Error("cache_get error",
			zap.Error(err),
			zap.String("key", key),
			zap.Int64("durationMs", duration.Milliseconds()))

		return nil, err
	}

	return json.RawMessage(result), nil
}

func (c *RPCCache) set(ctx context.Context, key, jsonRPCMethod string, value json.RawMessage, ttl time.Duration) {
	start := time.Now()
	cmd := c.writeClient.Set(ctx, key, string(value), ttl)
	duration := time.Since(start)

	err := cmd.Err()

	// Record metrics
	c.metricsContainer.CacheWriteDuration.
		WithLabelValues(jsonRPCMethod).
		Observe(duration.Seconds())

	zap.L().Debug("cache_set",
		zap.String("key", key),
		zap.String("jsonRPCMethod", jsonRPCMethod),
		zap.Any("value", value),
		zap.Int64("durationMs", duration.Milliseconds()),
		zap.Any("ttl", ttl))

	if err != nil {
		c.metricsContainer.CacheErrors.WithLabelValues("set").Inc()
		zap.L().Error("cache_set error",
			zap.Error(err),
			zap.String("key", key),
			zap.Any("value", value),
			zap.Int64("durationMs", duration.Milliseconds()),
			zap.Any("ttl", ttl))
	}
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

// Uses the go-redis/cache library
func (c *RPCCache) HandleRequest(chainName string, ttl time.Duration, reqBody jsonrpc.SingleRequestBody, originFunc func() (*jsonrpc.SingleResponseBody, error)) (json.RawMessage, bool, error) {
	var (
		// Technically could be a coalesced request as well.
		cached = true
		result json.RawMessage
	)

	// Counts requests in flight. If it's spiking that means that a lot of requests are for the same key.
	// If there's a too many requests in flight, and redis latency is high it means that the cache is down.
	c.metricsContainer.CacheRequestsInFlight.WithLabelValues(reqBody.Method).Inc()

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

// Non coalesced requests
// Uses the redis clients instead of the go-redis/cache library
func (c *RPCCache) HandleRequestParallel(
	chainName string,
	ttl time.Duration,
	reqBody jsonrpc.SingleRequestBody,
	originFunc func() (*jsonrpc.SingleResponseBody, error),
) (json.RawMessage, bool, error) {
	var (
		cached = true
	)

	ctx := context.Background()

	key := c.CreateRequestKey(chainName, reqBody)
	result, err := c.get(ctx, key, reqBody.Method) // Attempt to fetch from cache

	if err == nil {
		return result, cached, nil
	}

	// Cache miss or error, proceed with the request to origin
	cached = false
	respBody, err := originFunc()

	if err != nil {
		return nil, cached, err
	}

	result = respBody.Result

	// Perform cache set asynchronously
	go func() {
		c.set(
			ctx,
			key,
			reqBody.Method,
			result,
			ttl,
		)
	}()

	return result, cached, nil
}
