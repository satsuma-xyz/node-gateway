package checks

import (
	"context"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
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
	zap.L().Debug("Initializing PeerCheck.", zap.Any("upstreamID", c.upstreamConfig))

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

func (c *PeerCheck) RunCheck() {
	if c.client == nil {
		if err := c.Initialize(); err != nil {
			zap.L().Error("Error initializing PeerCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Error(err))
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

	peerCount, err := c.client.PeerCount(context.Background())
	c.peerCount = peerCount
	c.err = err

	zap.L().Debug("Ran PeerCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Any("peerCount", c.peerCount), zap.Error(c.err))
}

func (c *PeerCheck) IsPassing() bool {
	if c.shouldRun && (c.err != nil || c.peerCount < MinimumPeerCount) {
		zap.L().Debug("PeerCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("peerCount", c.peerCount), zap.Error(c.err))

		return false
	}

	return true
}
