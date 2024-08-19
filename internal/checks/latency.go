package checks

import (
	"context"
	"sync"
	"time"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

type FailureCounts struct {
	// TODO(polsar): Replace these with sliding window counts (must be thread-safe).
	// https://failsafe-go.dev/circuit-breaker/
	latencyTooHigh uint64
	timeoutOrError uint64
}

func NewFailureCounts() *FailureCounts {
	return &FailureCounts{
		latencyTooHigh: 0,
		timeoutOrError: 0,
	}
}

type LatencyCheck struct {
	client              client.EthClient
	Err                 error
	clientGetter        client.EthClientGetter
	metricsContainer    *metrics.Container
	logger              *zap.Logger
	upstreamConfig      *conf.UpstreamConfig
	routingConfig       *conf.RoutingConfig
	methodFailureCounts map[string]*FailureCounts // RPC method -> FailureCounts
	lock                sync.RWMutex
	ShouldRun           bool
}

func NewLatencyChecker(
	upstreamConfig *conf.UpstreamConfig,
	routingConfig *conf.RoutingConfig,
	clientGetter client.EthClientGetter,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) types.Checker {
	c := &LatencyCheck{
		upstreamConfig:      upstreamConfig,
		routingConfig:       routingConfig,
		clientGetter:        clientGetter,
		metricsContainer:    metricsContainer,
		logger:              logger,
		methodFailureCounts: make(map[string]*FailureCounts),
		ShouldRun:           routingConfig.Errors != nil || routingConfig.Latency != nil,
	}

	if err := c.Initialize(); err != nil {
		logger.Error("Error initializing LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Error(err))
	}

	return c
}

func (c *LatencyCheck) Initialize() error {
	c.logger.Debug("Initializing LatencyCheck.", zap.Any("config", c.upstreamConfig))

	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &c.upstreamConfig.BasicAuthConfig, &c.upstreamConfig.RequestHeadersConfig)
	if err != nil {
		c.Err = err
		return c.Err
	}

	c.client = httpClient

	c.runCheck()

	// TODO(polsar): This check is in both PeerCheck and SyncingCheck implementations, but I don't understand what it's supposed to be doing.
	// The setup is exactly the same in each case, so which method is not supported if the `isMethodNotSupportedErr` call returns `true`?
	if isMethodNotSupportedErr(c.Err) {
		c.logger.Debug("LatencyCheck is not supported by upstream, not running check.", zap.String("upstreamID", c.upstreamConfig.ID))

		c.ShouldRun = false
	}

	return nil
}

func (c *LatencyCheck) RunCheck() {
	if c.client == nil {
		if err := c.Initialize(); err != nil {
			c.logger.Error("Error initializing LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
			c.metricsContainer.LatencyCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPInit).Inc()
		}
	}

	if c.ShouldRun {
		c.runCheck()
	}
}

func (c *LatencyCheck) runCheck() {
	if c.client == nil {
		return
	}

	latencyConfig := c.routingConfig.Latency
	if latencyConfig == nil {
		// TODO(polsar): We still want to check the latency of LatencyCheckMethod using the top-level latency threshold.
		return
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	// Iterate over all (method, latencyThreshold) pairs and launch the check for each in parallel.
	// Note that `latencyConfig.MethodLatencyThresholds` is never modified after its initialization
	// in `config` package, so we don't need a lock to protect concurrent read access.
	for method, latencyThreshold := range latencyConfig.MethodLatencyThresholds {
		wg.Add(1)

		// Passing the loop variables as arguments is required to prevent the following lint error:
		// loopclosure: loop variable method captured by func literal (govet)
		go func(method string, latencyThreshold time.Duration) {
			defer wg.Done()

			runCheck := func() {
				c.runCheckForMethod(method, latencyThreshold)
			}

			runCheckWithMetrics(runCheck,
				c.metricsContainer.LatencyCheckRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
				c.metricsContainer.LatencyCheckDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
		}(method, latencyThreshold)
	}
}

// This method runs the latency check for the specified method and latency threshold.
func (c *LatencyCheck) runCheckForMethod(method string, latencyThreshold time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), RPCRequestTimeout)
	defer cancel()

	var val *FailureCounts

	func() {
		c.lock.Lock()
		defer c.lock.Unlock()

		var exists bool
		val, exists = c.methodFailureCounts[method]

		if !exists {
			// This is the first time we are checking this method so initialize its failure counts.
			//
			// TODO(polsar): Initialize all (method, FailureCounts) pairs in the Initialize method instead.
			// Once initialized, the map will only be read, eliminating the need for the lock.
			val = NewFailureCounts()
			c.methodFailureCounts[method] = val
		}
	}()

	// Make the request and increment the appropriate failure count if it takes too long or errors out.
	var duration time.Duration
	duration, c.Err = c.client.Latency(ctx, method)

	if c.Err != nil {
		val.timeoutOrError++

		c.metricsContainer.LatencyCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
	} else if duration > latencyThreshold {
		val.latencyTooHigh++

		c.metricsContainer.LatencyCheckHighLatencies.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
	}

	c.metricsContainer.Latency.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(duration.Milliseconds()))

	c.logger.Debug("Ran LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Any("latency", duration), zap.Error(c.Err))
}

func (c *LatencyCheck) IsPassing() bool {
	// TODO(polsar): Implement this method.
	return true
}
