package checks

import (
	"context"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

type PeerCheck struct {
	client           client.EthClient
	Err              error
	clientGetter     client.EthClientGetter
	metricsContainer *metrics.Container
	logger           *zap.Logger
	upstreamConfig   *conf.UpstreamConfig
	PeerCount        uint64
	ShouldRun        bool
}

func NewPeerChecker(
	upstreamConfig *conf.UpstreamConfig,
	clientGetter client.EthClientGetter,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) types.Checker {
	c := &PeerCheck{
		upstreamConfig:   upstreamConfig,
		clientGetter:     clientGetter,
		metricsContainer: metricsContainer,
		logger:           logger,
		// When we verify `peerCount` is a supported method of the Upstream, this may be set
		// to false if it's not supported.
		ShouldRun: upstreamConfig.HealthCheckConfig.SkipPeerCountCheck == nil || !*upstreamConfig.HealthCheckConfig.SkipPeerCountCheck,
	}

	if err := c.Initialize(); err != nil {
		logger.Error("Error initializing PeerCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Error(err))
	}

	return c
}

func (c *PeerCheck) Initialize() error {
	c.logger.Debug("Initializing PeerCheck.", zap.Any("config", c.upstreamConfig))

	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &c.upstreamConfig.BasicAuthConfig, &c.upstreamConfig.RequestHeadersConfig)
	if err != nil {
		c.Err = err
		return c.Err
	}

	c.client = httpClient

	c.runCheck()

	if isMethodNotSupportedErr(c.Err) {
		c.logger.Debug("PeerCheck is not supported by upstream, not running check.", zap.String("upstreamID", c.upstreamConfig.ID))

		c.ShouldRun = false
	}

	return nil
}

func (c *PeerCheck) RunCheck() {
	if c.client == nil {
		if err := c.Initialize(); err != nil {
			c.logger.Error("Error initializing PeerCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
			c.metricsContainer.PeerCountCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPInit).Inc()
		}
	}

	if c.ShouldRun {
		c.runCheck()
	}
}

func (c *PeerCheck) runCheck() {
	if c.client == nil {
		return
	}

	runCheck := func() {
		ctx, cancel := context.WithTimeout(context.Background(), RPCRequestTimeout)
		defer cancel()

		peerCount, err := c.client.PeerCount(ctx)
		if c.Err = err; c.Err != nil {
			c.metricsContainer.PeerCountCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
			return
		}

		c.PeerCount = peerCount
		c.metricsContainer.PeerCount.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(c.PeerCount))

		c.logger.Debug("Ran PeerCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Any("peerCount", c.PeerCount), zap.Error(c.Err))
	}

	runCheckWithMetrics(runCheck,
		c.metricsContainer.PeerCountCheckRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
		c.metricsContainer.PeerCountCheckDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
}

func (c *PeerCheck) IsPassing() bool {
	// TODO(polsar): This method is unused. Instead, the decision whether this check is passing is made here:
	//  https://github.com/satsuma-xyz/node-gateway/blob/b7f20aa2ad97f53772e9fa1565a300be7c0fff78/internal/route/node_filter.go#L61
	if c.ShouldRun && (c.Err != nil || c.PeerCount < MinimumPeerCount) {
		c.logger.Debug("PeerCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("peerCount", c.PeerCount), zap.Error(c.Err))

		return false
	}

	return true
}
