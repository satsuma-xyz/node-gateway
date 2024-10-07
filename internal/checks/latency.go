package checks

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
)

type LatencyCheck struct {
	Err                  error
	metricsContainer     *metrics.Container
	logger               *zap.Logger
	upstreamConfig       *config.UpstreamConfig
	routingConfig        *config.RoutingConfig
	methodLatencyBreaker map[string]LatencyCircuitBreaker // RPC method -> LatencyCircuitBreaker
	lock                 sync.RWMutex
	isCheckEnabled       bool
}

type LatencyCircuitBreaker interface {
	RecordLatency(latency time.Duration)
	IsOpen() bool
	GetThreshold() time.Duration
}

type LatencyStats struct {
	circuitBreaker circuitbreaker.CircuitBreaker[any]
	threshold      time.Duration
}

func (l *LatencyStats) RecordLatency(latency time.Duration) {
	if latency >= l.threshold {
		l.circuitBreaker.RecordFailure()
	} else {
		l.circuitBreaker.RecordSuccess()
	}
}

func (l *LatencyStats) GetThreshold() time.Duration {
	return l.threshold
}

func NewLatencyStats(routingConfig *config.RoutingConfig, method string) LatencyCircuitBreaker {
	return &LatencyStats{
		threshold: getLatencyThreshold(routingConfig, method),
		circuitBreaker: NewCircuitBreaker(
			config.DefaultLatencyTooHighRate,
			getDetectionWindow(routingConfig),
			getBanWindow(routingConfig),
		),
	}
}

func NewLatencyChecker(
	upstreamConfig *config.UpstreamConfig,
	routingConfig *config.RoutingConfig,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) types.ErrorLatencyChecker {
	return &LatencyCheck{
		upstreamConfig:       upstreamConfig,
		routingConfig:        routingConfig,
		metricsContainer:     metricsContainer,
		logger:               logger,
		methodLatencyBreaker: make(map[string]LatencyCircuitBreaker),
		isCheckEnabled:       routingConfig.IsCheckEnabled,
	}
}

// Returns the LatencyStats instance corresponding to the specified RPC method.
// This method is thread-safe.
func (c *LatencyCheck) getLatencyCircuitBreaker(method string) LatencyCircuitBreaker {
	c.lock.Lock()
	defer c.lock.Unlock()

	stats, exists := c.methodLatencyBreaker[method]

	if !exists {
		// This is the first time we are checking this method so initialize its LatencyStats instance.
		stats = NewLatencyStats(c.routingConfig, method)
		c.methodLatencyBreaker[method] = stats
	}

	return stats
}

func getLatencyThreshold(routingConfig *config.RoutingConfig, method string) time.Duration {
	if routingConfig != nil && routingConfig.Latency != nil {
		if latency, exists := routingConfig.Latency.MethodLatencyThresholds[method]; exists {
			return latency
		}

		return routingConfig.Latency.Threshold
	}

	return config.DefaultMaxLatency
}

func (l *LatencyStats) IsOpen() bool {
	// TODO(polsar): We should be able to check `l.circuitBreaker.IsOpen()`,
	//  but it appears to remain open forever, regardless of the configured delay.
	//  We also must reset the circuit breaker manually if it is not supposed to be open.
	isOpen := l.circuitBreaker.RemainingDelay() > 0
	if !isOpen {
		l.circuitBreaker.Close()
	}

	return isOpen
}

func (c *LatencyCheck) IsPassing(methods []string) bool {
	if !c.isCheckEnabled {
		return true
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.methodLatencyBreaker == nil {
		return true
	}

	// Only consider the passed methods, even if other methods' circuit breakers might be open.
	//
	// TODO(polsar): If the circuit breaker for any of the passed methods is open, we consider this upstream
	//  as unhealthy for all the other passed methods, even if their circuit breakers are closed. This might
	//  be undesirable, though since all the methods are part of the same request, we would have to somehow
	//  modify the request to only contain the healthy methods. This seems like more trouble than is worth.
	for _, method := range methods {
		if breaker, exists := c.methodLatencyBreaker[method]; exists && breaker.IsOpen() {
			c.logger.Debug(
				"ErrorLatencyCheck is not passing due to high latency of an RPC method.",
				zap.String("upstreamID", c.upstreamConfig.ID),
				zap.Any("method", method),
				zap.Error(c.Err),
			)

			return false
		}
	}

	return true
}

func (c *LatencyCheck) RecordRequest(data *types.RequestData) {
	if !c.isCheckEnabled {
		return
	}

	// Record the request latency if latency checking is enabled.
	if c.methodLatencyBreaker != nil {
		latencyCircuitBreaker := c.getLatencyCircuitBreaker(data.Method)
		latencyCircuitBreaker.RecordLatency(data.Latency)

		if data.Latency >= latencyCircuitBreaker.GetThreshold() {
			c.metricsContainer.ErrorLatencyCheckHighLatencies.WithLabelValues(
				c.upstreamConfig.ID,
				c.upstreamConfig.HTTPURL,
				metrics.HTTPRequest,
				data.Method,
			).Inc()
		}
	}

	c.metricsContainer.ErrorLatency.WithLabelValues(
		c.upstreamConfig.ID,
		c.upstreamConfig.HTTPURL,
		data.Method,
	).Set(float64(data.Latency.Milliseconds()))
}
