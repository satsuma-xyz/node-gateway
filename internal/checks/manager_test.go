package checks

import (
	"testing"
	"time"

	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheckManager(t *testing.T) {
	ethereumClient := mocks.NewEthClient(t)
	mockEthClientGetter := func(url string, credentials *config.BasicAuthConfig, additionalRequestHeaders *[]config.RequestHeaderConfig) (client.EthClient, error) { //nolint:gocritic,nolintlint,revive
		return ethereumClient, nil
	}

	mockBlockHeightChecker := mocks.NewBlockHeightChecker(t)
	mockPeerChecker := mocks.NewChecker(t)
	mockSyncingChecker := mocks.NewChecker(t)

	mockBlockHeightChecker.Mock.On("RunCheck").Return(nil)
	mockPeerChecker.Mock.On("RunCheck").Return(nil)
	mockSyncingChecker.Mock.On("RunCheck").Return(nil)

	configs := []config.UpstreamConfig{
		{
			ID:                "mainnet",
			HTTPURL:           "http://rpc.ankr.io/eth",
			WSURL:             "wss://something/something",
			HealthCheckConfig: config.HealthCheckConfig{UseWSForBlockHeight: new(bool)},
		},
	}

	tickerChan := make(chan time.Time)
	ticker := &time.Ticker{C: tickerChan}

	metricsContainer := metrics.NewContainer(config.TestChainName)

	manager := NewHealthCheckManager(mockEthClientGetter, configs, nil, ticker, metricsContainer, zap.L())
	manager.(*healthCheckManager).newBlockHeightCheck = func(
		*config.UpstreamConfig,
		client.EthClientGetter,
		BlockHeightObserver,
		*metrics.Container,
		*zap.Logger,
	) types.BlockHeightChecker {
		return mockBlockHeightChecker
	}
	manager.(*healthCheckManager).newPeerCheck = func(
		upstreamConfig *config.UpstreamConfig, //nolint:nolintlint,revive // Legacy
		clientGetter client.EthClientGetter, //nolint:nolintlint,revive // Legacy
		metricsContainer *metrics.Container, //nolint:nolintlint,revive // Legacy
		logger *zap.Logger, //nolint:nolintlint,revive // Legacy
	) types.Checker {
		return mockPeerChecker
	}
	manager.(*healthCheckManager).newSyncingCheck = func(
		upstreamConfig *config.UpstreamConfig, //nolint:nolintlint,revive // Legacy
		clientGetter client.EthClientGetter, //nolint:nolintlint,revive // Legacy
		metricsContainer *metrics.Container, //nolint:nolintlint,revive // Legacy
		logger *zap.Logger, //nolint:nolintlint,revive // Legacy
	) types.Checker {
		return mockSyncingChecker
	}

	manager.StartHealthChecks()

	assert.Eventually(t, func() bool {
		return len(mockBlockHeightChecker.Calls) >= 1
	}, 1*time.Second, time.Millisecond)

	mockPeerChecker.AssertNumberOfCalls(t, "RunCheck", 1)
	mockSyncingChecker.AssertNumberOfCalls(t, "RunCheck", 1)
	mockBlockHeightChecker.AssertNumberOfCalls(t, "RunCheck", 1)

	tickerChan <- time.Now()

	assert.Eventually(t, func() bool {
		return len(mockBlockHeightChecker.Calls) >= 2
	}, 1*time.Second, time.Millisecond)

	mockPeerChecker.AssertNumberOfCalls(t, "RunCheck", 2)
	mockSyncingChecker.AssertNumberOfCalls(t, "RunCheck", 2)
	mockBlockHeightChecker.AssertNumberOfCalls(t, "RunCheck", 2)
}
