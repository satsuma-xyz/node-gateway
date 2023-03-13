package checks

import (
	"context"
	"errors"

	ethTypes "github.com/ethereum/go-ethereum/core/types"
	internalTypes "github.com/satsuma-data/node-gateway/internal/types"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"go.uber.org/zap"
)

type BlockHeightCheck struct {
	httpClient          client.EthClient
	webSocketError      error
	blockHeightError    error
	clientGetter        client.EthClientGetter
	upstreamConfig      *conf.UpstreamConfig
	blockHeightObserver BlockHeightObserver
	metricsContainer    *metrics.Container
	logger              *zap.Logger
	blockHeight         uint64
	useWSForBlockHeight bool
}

type BlockHeightObserver interface {
	ProcessBlockHeightUpdate(groupID string, upstreamID string, blockHeight uint64)
	ProcessErrorUpdate(groupID string, upstreamID string, err error)
}

func NewBlockHeightChecker(
	config *conf.UpstreamConfig,
	clientGetter client.EthClientGetter,
	blockHeightObserver BlockHeightObserver,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) internalTypes.BlockHeightChecker {
	c := &BlockHeightCheck{
		upstreamConfig:      config,
		clientGetter:        clientGetter,
		blockHeightObserver: blockHeightObserver,
		metricsContainer:    metricsContainer,
		logger:              logger,
	}

	c.Initialize()

	return c
}

func (c *BlockHeightCheck) Initialize() {
	c.logger.Debug("Initializing BlockHeightCheck.", zap.Any("config", c.upstreamConfig))

	if err := c.initializeWebsockets(); err != nil {
		c.logger.Error("Encountered error when calling SubscribeNewHead over Websockets, falling back to using HTTP polling for BlockHeightCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))
	}

	c.initializeHTTP()
}

func (c *BlockHeightCheck) initializeWebsockets() error {
	if c.upstreamConfig.WSURL != "" &&
		(c.upstreamConfig.HealthCheckConfig.UseWSForBlockHeight == nil || *c.upstreamConfig.HealthCheckConfig.UseWSForBlockHeight) {
		c.logger.Debug("Subscribing over Websockets for BlockHeightCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))

		c.useWSForBlockHeight = true

		return c.subscribeNewHead()
	}

	c.logger.Debug("Not subscribing over Websockets for BlockHeightCheck.", zap.Any("upstreamID", c.upstreamConfig.ID))

	return nil
}

func (c *BlockHeightCheck) initializeHTTP() {
	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		c.metricsContainer.BlockHeightCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPInit).Inc()
		c.setError(err)

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
		c.logger.Debug("Not running BlockHeightCheck over HTTP, Websockets subscription still active.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))
	}
}

func (c *BlockHeightCheck) runCheckHTTP() {
	if c.httpClient == nil {
		return
	}

	runCheck := func() {
		ctx, cancel := context.WithTimeout(context.Background(), RPCRequestTimeout)
		defer cancel()

		header, err := c.httpClient.HeaderByNumber(ctx, nil)

		if c.blockHeightError = err; c.blockHeightError != nil {
			c.metricsContainer.BlockHeightCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
			return
		}

		c.SetBlockHeight(header.Number.Uint64())

		c.metricsContainer.BlockHeight.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(c.blockHeight))

		c.logger.Debug("Ran BlockHeightCheck over HTTP.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("httpURL", c.upstreamConfig.HTTPURL), zap.Uint64("blockHeight", c.blockHeight))
	}

	runCheckWithMetrics(runCheck,
		c.metricsContainer.BlockHeightCheckRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
		c.metricsContainer.BlockHeightCheckDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
}

func (c *BlockHeightCheck) IsPassing(maxBlockHeight uint64) bool {
	if c.blockHeightError != nil || c.blockHeight < maxBlockHeight {
		c.logger.Debug("BlockHeightCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("blockHeight", c.blockHeight), zap.Error(c.blockHeightError))

		return false
	}

	return true
}

func (c *BlockHeightCheck) GetBlockHeight() uint64 {
	return c.blockHeight
}

func (c *BlockHeightCheck) SetBlockHeight(blockHeight uint64) {
	c.blockHeight = blockHeight
	c.blockHeightObserver.ProcessBlockHeightUpdate(c.upstreamConfig.GroupID, c.upstreamConfig.ID, blockHeight)
}

func (c *BlockHeightCheck) GetError() error {
	return c.blockHeightError
}

func (c *BlockHeightCheck) setError(err error) {
	c.blockHeightError = err
	c.blockHeightObserver.ProcessErrorUpdate(c.upstreamConfig.GroupID, c.upstreamConfig.ID, err)
}

func (c *BlockHeightCheck) subscribeNewHead() error {
	onNewHead := func(header *ethTypes.Header) {
		c.SetBlockHeight(header.Number.Uint64())

		c.logger.Debug("Received blockheight over Websockets.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("httpURL", c.upstreamConfig.HTTPURL), zap.Uint64("blockHeight", c.blockHeight))
		c.metricsContainer.BlockHeight.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(c.blockHeight))

		c.webSocketError = nil
		c.setError(nil)
	}

	onError := func(failure string) {
		c.logger.Error("Encountered error in NewHead Websockets subscription.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))

		c.metricsContainer.BlockHeightCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.WSError).Inc()
		c.webSocketError = errors.New(failure)
		c.setError(c.webSocketError)
	}

	wsClient, err := c.clientGetter(c.upstreamConfig.WSURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		c.webSocketError = err
		return err
	}

	if err = subscribeNewHeads(wsClient, &newHeadHandler{onNewHead: onNewHead, onError: onError}); err != nil {
		c.metricsContainer.BlockHeightCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.WSSubscribe).Inc()
		c.webSocketError = err

		return err
	}

	return nil
}

type newHeadHandler struct {
	onNewHead func(header *ethTypes.Header)
	onError   func(failure string)
}

func subscribeNewHeads(wsClient client.EthClient, handler *newHeadHandler) error {
	ctx, cancel := context.WithTimeout(context.Background(), RPCRequestTimeout)
	defer cancel()

	ch := make(chan *ethTypes.Header)

	subscription, err := wsClient.SubscribeNewHead(ctx, ch)

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
