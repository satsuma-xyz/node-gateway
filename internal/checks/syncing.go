package checks

import (
	"context"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"go.uber.org/zap"
)

type SyncingCheck struct {
	client         client.EthClient
	err            error
	clientGetter   client.EthClientGetter
	upstreamConfig *conf.UpstreamConfig
	isSyncing      bool
	shouldRun      bool
}

func NewSyncingChecker(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter) Checker {
	c := &SyncingCheck{
		upstreamConfig: upstreamConfig,
		clientGetter:   clientGetter,
		// Set `isSyncing:true` until we check the upstream node's syncing status.
		isSyncing: true,
		// Set `ShouldRun:true` until we verify `eth.syncing` is a supported method of the Upstream.
		shouldRun: true,
	}

	if err := c.Initialize(); err != nil {
		zap.L().Error("Error initializing SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
	}

	return c
}

func (c *SyncingCheck) Initialize() error {
	zap.L().Debug("Initializing SyncingCheck.", zap.Any("config", c.upstreamConfig))

	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		c.err = err
		return c.err
	}

	c.client = httpClient

	c.runCheck()

	if isMethodNotSupportedErr(c.err) {
		zap.L().Debug("PeerCheck is not supported by upstream, not running check.", zap.Any("upstreamID", c.upstreamConfig.ID))

		c.shouldRun = false
	}

	return nil
}

func (c *SyncingCheck) RunCheck() {
	if c.client == nil {
		if err := c.Initialize(); err != nil {
			zap.L().Error("Error initializing SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
			metrics.SyncStatusCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Inc()
		}
	}

	if c.shouldRun {
		c.runCheck()
	}
}

func (c *SyncingCheck) runCheck() {
	if c.client == nil {
		return
	}

	runCheck := func() {
		result, err := c.client.SyncProgress(context.Background())
		if c.err = err; err != nil {
			metrics.SyncStatusCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Inc()
			return
		}

		c.isSyncing = result != nil

		gauge := 0
		if c.isSyncing {
			gauge = 1
		}

		metrics.SyncStatus.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(gauge))

		zap.L().Debug("Ran SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Any("syncProgress", result))
	}

	runCheckWithMetrics(runCheck,
		metrics.SyncStatusCheckRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
		metrics.SyncStatusCheckDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
}

func (c *SyncingCheck) IsPassing() bool {
	if c.shouldRun && (c.isSyncing || c.err != nil) {
		zap.L().Error("SyncingCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("isSyncing", c.isSyncing), zap.Error(c.err))

		return false
	}

	return true
}
