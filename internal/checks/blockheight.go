package checks

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"go.uber.org/zap"
)

//go:generate mockery --output ../mocks --name BlockHeightChecker
type BlockHeightChecker interface {
	RunCheck()
	GetBlockHeight() uint64
	IsPassing(maxBlockHeight uint64) bool
}

type BlockHeightCheck struct {
	httpClient          client.EthClient
	WebSocketError      error
	BlockHeightError    error
	clientGetter        client.EthClientGetter
	upstreamConfig      *conf.UpstreamConfig
	BlockHeight         uint64
	UseWSForBlockHeight bool
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
	if err := c.initializeWebsockets(); err != nil {
		zap.L().Error("Encountered error when calling SubscribeNewHead over Websockets, falling back to using HTTP polling.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))
	}

	c.initializeHTTP()
}

func (c *BlockHeightCheck) initializeWebsockets() error {
	if c.upstreamConfig.WSURL != "" &&
		(c.upstreamConfig.HealthCheckConfig.UseWSForBlockHeight == nil || *c.upstreamConfig.HealthCheckConfig.UseWSForBlockHeight) {
		zap.L().Debug("Subscribing over Websockets to check block height.", zap.Any("upstreamID", c.upstreamConfig.ID))

		c.UseWSForBlockHeight = true

		return c.subscribeNewHead()
	}

	zap.L().Debug("Not subscribing over Websockets to check block height.", zap.Any("upstreamID", c.upstreamConfig.ID))

	return nil
}

func (c *BlockHeightCheck) initializeHTTP() {
	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		c.BlockHeightError = err
		return
	}

	c.httpClient = httpClient
}

func (c *BlockHeightCheck) RunCheck() {
	if !c.UseWSForBlockHeight || (c.UseWSForBlockHeight && c.WebSocketError != nil) {
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

	header, err := c.httpClient.HeaderByNumber(context.Background(), nil)
	if c.BlockHeightError = err; c.BlockHeightError != nil {
		return
	}

	c.BlockHeight = header.Number.Uint64()

	zap.L().Debug("Ran BlockHeightCheck over HTTP.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL), zap.Uint64("blockHeight", c.BlockHeight))
}

func (c *BlockHeightCheck) IsPassing(maxBlockHeight uint64) bool {
	if c.BlockHeightError != nil && c.BlockHeight < maxBlockHeight {
		zap.L().Debug("BlockHeightCheck is not passing.", zap.String("upstreamID", c.upstreamConfig.ID), zap.Any("blockHeight", c.BlockHeight), zap.Error(c.BlockHeightError))

		return false
	}

	return true
}

func (c *BlockHeightCheck) GetBlockHeight() uint64 {
	return c.BlockHeight
}

func (c *BlockHeightCheck) subscribeNewHead() error {
	onNewHead := func(header *types.Header) {
		c.BlockHeight = header.Number.Uint64()
		c.WebSocketError = nil
		c.BlockHeightError = nil
	}

	onError := func(failure string) {
		zap.L().Error("Encountered error in NewHead Websockets subscription.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.String("WSURL", c.upstreamConfig.WSURL))

		c.WebSocketError = errors.New(failure)
		c.BlockHeightError = c.WebSocketError
	}

	wsClient, err := c.clientGetter(c.upstreamConfig.WSURL, &client.BasicAuthCredentials{Username: c.upstreamConfig.BasicAuthConfig.Username, Password: c.upstreamConfig.BasicAuthConfig.Password})
	if err != nil {
		c.WebSocketError = err
		return err
	}

	if err = subscribeNewHeads(wsClient, &newHeadHandler{onNewHead: onNewHead, onError: onError}); err != nil {
		c.WebSocketError = err
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
