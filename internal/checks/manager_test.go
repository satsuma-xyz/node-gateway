package checks

import (
	"testing"
	"time"

	"github.com/satsuma-data/node-gateway/internal/types"

	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHealthCheckManager(t *testing.T) {
	ethereumClient := mocks.NewEthClient(t)
	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethereumClient, nil
	}

	mockBlockHeightChecker := mocks.NewBlockHeightChecker(t)
	mockPeerChecker := mocks.NewChecker(t)
	mockSyncingChecker := mocks.NewChecker(t)

	mockBlockHeightChecker.Mock.On("GetError").Return(nil)
	mockBlockHeightChecker.Mock.On("IsPassing", mock.Anything).Return(true)
	mockBlockHeightChecker.Mock.On("RunCheck").Return(nil)
	mockBlockHeightChecker.Mock.On("GetBlockHeight").Return(uint64(5))
	mockPeerChecker.Mock.On("IsPassing").Return(true)
	mockPeerChecker.Mock.On("RunCheck").Return(nil)
	mockSyncingChecker.Mock.On("IsPassing").Return(true)
	mockSyncingChecker.Mock.On("RunCheck").Return(nil)

	configs := []config.UpstreamConfig{
		{
			ID:                "mainnet",
			HTTPURL:           "http://rpc.ankr.io/eth",
			WSURL:             "wss://something/something",
			HealthCheckConfig: config.HealthCheckConfig{UseWSForBlockHeight: new(bool)},
		},
	}

	manager := NewHealthCheckManager(mockEthClientGetter, configs, nil)
	manager.(*healthCheckManager).newBlockHeightCheck = func(config *config.UpstreamConfig, clientGetter client.EthClientGetter, blockHeightObserver chan<- uint64) types.BlockHeightChecker {
		return mockBlockHeightChecker
	}
	manager.(*healthCheckManager).newPeerCheck = func(upstreamConfig *config.UpstreamConfig, clientGetter client.EthClientGetter) types.Checker {
		return mockPeerChecker
	}
	manager.(*healthCheckManager).newSyncingCheck = func(upstreamConfig *config.UpstreamConfig, clientGetter client.EthClientGetter) types.Checker {
		return mockSyncingChecker
	}

	manager.StartHealthChecks()

	assert.Eventually(t, func() bool {
		mainnetStatus := manager.(*healthCheckManager).GetUpstreamStatus("mainnet")
		return mainnetStatus.IsHealthy(5)
	}, 2*time.Second, 10*time.Millisecond, "Healthy upstreams did not include expected values.")

	mockBlockHeightChecker.ExpectedCalls = nil
	mockBlockHeightChecker.Calls = nil
	mockBlockHeightChecker.Mock.On("IsPassing", mock.Anything).Return(false)

	// Verify that no healthy upstreams are returned after a check starts failing.
	assert.Eventually(t, func() bool {
		mainnetStatus := manager.(*healthCheckManager).GetUpstreamStatus("mainnet")
		return mainnetStatus.IsHealthy(5) == false
	}, 2*time.Second, 10*time.Millisecond, "Found healthy upstreams when expected none.")
	mockBlockHeightChecker.AssertNotCalled(t, "GetBlockHeight")
}
