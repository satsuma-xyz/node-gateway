package checks

import (
	"context"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

type SyncingCheck struct {
	client           client.EthClient
	Err              error
	clientGetter     client.EthClientGetter
	metricsContainer *metrics.Container
	logger           *zap.Logger
	upstreamConfig   *conf.UpstreamConfig
	IsSyncing        bool
	ShouldRun        bool
}

func NewSyncingChecker(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter, metricsContainer *metrics.Container, logger *zap.Logger) types.Checker {
	c := &SyncingCheck{
		upstreamConfig:   upstreamConfig,
		clientGetter:     clientGetter,
		metricsContainer: metricsContainer,
		logger:           logger,
		// Set `isSyncing:true` until we check the upstream node's syncing status.
		IsSyncing: true,
		// Set `ShouldRun:true` until we verify `eth.syncing` is a supported method of the Upstream.
		ShouldRun: true,
	}

	if err := c.Initialize(); err != nil {
		logger.Error("Error initializing SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
	}

	return c
}

func (c *SyncingCheck) Initialize() error {
	c.logger.Debug("Initializing SyncingCheck.", zap.Any("config", c.upstreamConfig))

	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &c.upstreamConfig.BasicAuthConfig, &c.upstreamConfig.RequestHeadersConfig)
	if err != nil {
		c.Err = err
		return c.Err
	}

	c.client = httpClient

	c.runCheck()

	if isMethodNotSupportedErr(c.Err) {
		c.logger.Debug("SyncingCheck is not supported by upstream, not running check.", zap.Any("upstreamID", c.upstreamConfig.ID))

		c.ShouldRun = false
	}

	return nil
}

func (c *SyncingCheck) RunCheck() {
	if c.client == nil {
		if err := c.Initialize(); err != nil {
			c.logger.Error("Error initializing SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
			c.metricsContainer.SyncStatusCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPInit).Inc()
		}
	}

	if c.ShouldRun {
		c.runCheck()
	}
}

func (c *SyncingCheck) runCheck() {
	if c.client == nil {
		return
	}

	runCheck := func() {
		ctx, cancel := context.WithTimeout(context.Background(), RPCRequestTimeout)
		defer cancel()

		result, err := c.client.SyncProgress(ctx)
		if c.Err = err; err != nil {
			c.metricsContainer.SyncStatusCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
			return
		}

		c.IsSyncing = result != nil

		gauge := 0
		if c.IsSyncing {
			gauge = 1
		}

		c.metricsContainer.SyncStatus.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(gauge))

		c.logger.Debug("Ran SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Any("syncProgress", result))
	}

	runCheckWithMetrics(runCheck,
		c.metricsContainer.SyncStatusCheckRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
		c.metricsContainer.SyncStatusCheckDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
}

func (c *SyncingCheck) IsPassing() bool {
	// TODO(polsar): This method is unused. Instead, the decision whether this check is passing is made here:
	//  https://github.com/satsuma-xyz/node-gateway/blob/b7f20aa2ad97f53772e9fa1565a300be7c0fff78/internal/route/node_filter.go#L98
	if c.ShouldRun && (c.IsSyncing || c.Err != nil) {
		c.logger.Error("SyncingCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("isSyncing", c.IsSyncing), zap.Error(c.Err))

		return false
	}

	return true
}
