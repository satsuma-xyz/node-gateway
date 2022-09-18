package checks

import (
	"testing"
	"time"

	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/satsuma-data/node-gateway/internal/types"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheckManager(t *testing.T) {
	ethereumClient := mocks.NewEthClient(t)
	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
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
	//ticker := time.NewTicker(5 * time.Second)

	manager := NewHealthCheckManager(mockEthClientGetter, configs, nil, ticker)
	manager.(*healthCheckManager).newBlockHeightCheck = func(
		*config.UpstreamConfig,
		client.EthClientGetter,
		BlockHeightObserver,
	) types.BlockHeightChecker {
		return mockBlockHeightChecker
	}
	manager.(*healthCheckManager).newPeerCheck = func(
		upstreamConfig *config.UpstreamConfig,
		clientGetter client.EthClientGetter,
	) types.Checker {
		return mockPeerChecker
	}
	manager.(*healthCheckManager).newSyncingCheck = func(
		upstreamConfig *config.UpstreamConfig,
		clientGetter client.EthClientGetter,
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
