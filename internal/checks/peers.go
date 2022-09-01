package checks

import (
	"context"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"go.uber.org/zap"
)

type PeerCheck struct {
	client         client.EthClient
	err            error
	clientGetter   client.EthClientGetter
	upstreamConfig *conf.UpstreamConfig
	peerCount      uint64
	shouldRun      bool
}

func NewPeerChecker(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter) Checker {
	c := &PeerCheck{
		upstreamConfig: upstreamConfig,
		clientGetter:   clientGetter,
		// Set `ShouldRun:true` until we verify `peerCount` is a supported method of the Upstream.
		shouldRun: true,
	}

	if err := c.Initialize(); err != nil {
		zap.L().Error("Error initializing PeerCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Error(err))
	}

	return c
}

func (c *PeerCheck) Initialize() error {
	zap.L().Debug("Initializing PeerCheck.", zap.Any("config", c.upstreamConfig))

	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		c.err = err
		return c.err
	}

	c.client = httpClient

	c.runCheck()

	if isMethodNotSupportedErr(c.err) {
		zap.L().Debug("PeerCheck is not supported by upstream, not running check.", zap.String("upstreamID", c.upstreamConfig.ID))

		c.shouldRun = false
	}

	return nil
}

func (c *PeerCheck) RunCheck() {
	if c.client == nil {
		if err := c.Initialize(); err != nil {
			zap.L().Error("Errorr initializing PeerCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
			metrics.PeerCountErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Inc()
		}
	}

	if c.shouldRun {
		c.runCheck()
	}
}

func (c *PeerCheck) runCheck() {
	if c.client == nil {
		return
	}

	runCheck := func() {
		peerCount, err := c.client.PeerCount(context.Background())
		if c.err = err; c.err != nil {
			metrics.PeerCountErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Inc()
			return
		}

		c.peerCount = peerCount
		metrics.PeerCount.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(c.peerCount))

		zap.L().Debug("Ran PeerCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Any("peerCount", c.peerCount), zap.Error(c.err))
	}

	runCheckWithMetrics(runCheck,
		metrics.PeerCountTotalRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
		metrics.PeerCountDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
}

func (c *PeerCheck) IsPassing() bool {
	if c.shouldRun && (c.err != nil || c.peerCount < MinimumPeerCount) {
		zap.L().Debug("PeerCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("peerCount", c.peerCount), zap.Error(c.err))

		return false
	}

	return true
}
