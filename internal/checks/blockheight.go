package checks

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"go.uber.org/zap"
)

//go:generate mockery --output ../mocks --name BlockHeightChecker
type BlockHeightChecker interface {
	RunCheck()
	GetError() error
	GetBlockHeight() uint64
	IsPassing(maxBlockHeight uint64) bool
}

type BlockHeightCheck struct {
	httpClient          client.EthClient
	webSocketError      error
	blockHeightError    error
	clientGetter        client.EthClientGetter
	upstreamConfig      *conf.UpstreamConfig
	blockHeight         uint64
	useWSForBlockHeight bool
}

func NewBlockHeightChecker(config *conf.UpstreamConfig, clientGetter client.EthClientGetter) BlockHeightChecker {
	c := &BlockHeightCheck{
		upstreamConfig: config,
		clientGetter:   clientGetter,
	}

	c.Initialize()

	return c
}

func (c *BlockHeightCheck) Initialize() {
	zap.L().Debug("Initializing BlockHeightCheck.", zap.Any("config", c.upstreamConfig))

	if err := c.initializeWebsockets(); err != nil {
		zap.L().Error("Encountered error when calling SubscribeNewHead over Websockets, falling back to using HTTP polling for BlockHeightCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))
	}

	c.initializeHTTP()
}

func (c *BlockHeightCheck) initializeWebsockets() error {
	if c.upstreamConfig.WSURL != "" &&
		(c.upstreamConfig.HealthCheckConfig.UseWSForBlockHeight == nil || *c.upstreamConfig.HealthCheckConfig.UseWSForBlockHeight) {
		zap.L().Debug("Subscribing over Websockets for BlockHeightCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))

		c.useWSForBlockHeight = true

		return c.subscribeNewHead()
	}

	zap.L().Debug("Not subscribing over Websockets for BlockHeightCheck.", zap.Any("upstreamID", c.upstreamConfig.ID))

	return nil
}

func (c *BlockHeightCheck) initializeHTTP() {
	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		metrics.BlockHeightCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.BlockHeightCheckErrorTypeHTTP).Inc()
		c.blockHeightError = err

		return
	}

	c.httpClient = httpClient
}

func (c *BlockHeightCheck) RunCheck() {
	if !c.useWSForBlockHeight || (c.useWSForBlockHeight && c.webSocketError != nil) {
		if c.httpClient == nil {
			c.initializeHTTP()
		}

		c.runCheckHTTP()
	} else {
		zap.L().Debug("Not running BlockHeightCheck over HTTP, Websockets subscription still active.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))
	}
}

func (c *BlockHeightCheck) runCheckHTTP() {
	if c.httpClient == nil {
		return
	}

	runCheck := func() {
		header, err := c.httpClient.HeaderByNumber(context.Background(), nil)
		if c.blockHeightError = err; c.blockHeightError != nil {
			metrics.BlockHeightCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.BlockHeightCheckErrorTypeHTTP).Inc()
			return
		}

		c.blockHeight = header.Number.Uint64()
		metrics.BlockHeight.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(c.blockHeight))

		zap.L().Debug("Ran BlockHeightCheck over HTTP.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("httpURL", c.upstreamConfig.HTTPURL), zap.Uint64("blockHeight", c.blockHeight))
	}

	runCheckWithMetrics(runCheck,
		metrics.BlockHeightCheckRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
		metrics.BlockHeightCheckDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
}

func (c *BlockHeightCheck) IsPassing(maxBlockHeight uint64) bool {
	if c.blockHeightError != nil || c.blockHeight < maxBlockHeight {
		zap.L().Debug("BlockHeightCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("blockHeight", c.blockHeight), zap.Error(c.blockHeightError))

		return false
	}

	return true
}

func (c *BlockHeightCheck) GetBlockHeight() uint64 {
	return c.blockHeight
}

func (c *BlockHeightCheck) GetError() error {
	return c.blockHeightError
}

func (c *BlockHeightCheck) subscribeNewHead() error {
	onNewHead := func(header *types.Header) {
		c.blockHeight = header.Number.Uint64()
		metrics.BlockHeight.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(c.blockHeight))
		c.webSocketError = nil
		c.blockHeightError = nil
	}

	onError := func(failure string) {
		zap.L().Error("Encountered error in NewHead Websockets subscription.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))

		metrics.BlockHeightCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.BlockHeightCheckErrorTypeWSError).Inc()
		c.webSocketError = errors.New(failure)
		c.blockHeightError = c.webSocketError
	}

	wsClient, err := c.clientGetter(c.upstreamConfig.WSURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		c.webSocketError = err
		return err
	}

	if err = subscribeNewHeads(wsClient, &newHeadHandler{onNewHead: onNewHead, onError: onError}); err != nil {
		metrics.BlockHeightCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.BlockHeightCheckErrorTypeWSSubscribe).Inc()
		c.webSocketError = err

		return err
	}

	return nil
}

type newHeadHandler struct {
	onNewHead func(header *types.Header)
	onError   func(failure string)
}

func subscribeNewHeads(wsClient client.EthClient, handler *newHeadHandler) error {
	ch := make(chan *types.Header)
	subscription, err := wsClient.SubscribeNewHead(context.Background(), ch)

	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case header := <-ch:
				handler.onNewHead(header)

			case err := <-subscription.Err():
				if err != nil {
					handler.onError(err.Error())
				}

				return
			}
		}
	}()

	return nil
}
