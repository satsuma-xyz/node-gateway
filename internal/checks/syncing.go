package checks

import (
	"context"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
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
		zap.L().Error("Error initializing SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Error(err))
	}

	return c
}

func (c *SyncingCheck) Initialize() error {
	zap.L().Debug("Initializing SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig))

	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		c.err = err
		return c.err
	}

	c.client = httpClient

	c.runCheck()

	if isMethodNotSupportedErr(c.err) {
		c.shouldRun = false
	}

	return nil
}

func (c *SyncingCheck) RunCheck() {
	if c.client == nil {
		if err := c.Initialize(); err != nil {
			zap.L().Error("Error initializing SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Error(err))
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

	syncProgress, err := c.client.SyncProgress(context.Background())
	c.err = err
	c.isSyncing = syncProgress != nil

	zap.L().Debug("Ran SyncingCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Any("syncProgress", syncProgress), zap.Error(err))
}

func (c *SyncingCheck) IsPassing() bool {
	if c.shouldRun && (c.isSyncing || c.err != nil) {
		zap.L().Error("SyncingCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("peerCount", c.isSyncing), zap.Error(c.err))

		return false
	}

	return true
}
