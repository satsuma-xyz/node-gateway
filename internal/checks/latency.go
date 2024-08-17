package checks

import (
	"context"
	"time"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

const (
	// LatencyCheckMethod is a dummy method we use to measure the latency of an upstream RPC endpoint.
	// https://docs.infura.io/api/networks/ethereum/json-rpc-methods/eth_chainid
	LatencyCheckMethod = "eth_chainId"
	// If the latency threshold is not specified in the config, we use this value.
	defaultMaxLatency = RPCRequestTimeout
)

type FailureCounts struct {
	// TODO(polsar): Replace these with sliding window counts.
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
	methodFailureCounts map[string]*FailureCounts // RPC method -> FailureCounts
	ShouldRun           bool
}

func NewLatencyChecker(
	upstreamConfig *conf.UpstreamConfig,
	clientGetter client.EthClientGetter,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) types.Checker {
	c := &LatencyCheck{
		upstreamConfig:      upstreamConfig,
		clientGetter:        clientGetter,
		metricsContainer:    metricsContainer,
		logger:              logger,
		methodFailureCounts: make(map[string]*FailureCounts),
		ShouldRun:           true, // TODO(polsar): Set this from the config.
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

	runCheck := func() {
		ctx, cancel := context.WithTimeout(context.Background(), RPCRequestTimeout)
		defer cancel()

		// TODO(polsar): Add support for checking the latency of specific method(s), as specified in the config.
		method := LatencyCheckMethod

		// TODO(polsar): Protect the map with a mutex or use a thread-safe map.
		var val *FailureCounts
		val, exists := c.methodFailureCounts[method]

		if !exists {
			// This is the first time we are checking this method so initialize its failure counts.
			val = NewFailureCounts()
			c.methodFailureCounts[method] = val
		}

		// Make the request and increment the appropriate failure count if it takes too long or errors out.
		var duration time.Duration
		duration, c.Err = c.client.Latency(ctx, method)

		if c.Err != nil {
			val.timeoutOrError++

			c.metricsContainer.LatencyCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
		} else if duration > defaultMaxLatency { // TODO(polsar): Get the latency threshold from config.
			val.latencyTooHigh++

			c.metricsContainer.LatencyCheckHighLatencies.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
		}

		c.metricsContainer.Latency.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(duration.Milliseconds()))

		c.logger.Debug("Ran LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Any("latency", duration), zap.Error(c.Err))
	}

	runCheckWithMetrics(runCheck,
		c.metricsContainer.LatencyCheckRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
		c.metricsContainer.LatencyCheckDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
}

func (c *LatencyCheck) IsPassing() bool {
	// TODO(polsar): Implement this method.
	return true
}
