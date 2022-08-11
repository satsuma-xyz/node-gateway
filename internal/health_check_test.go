package internal

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHealthCheckConfig(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		config  HealthCheckConfig
		isValid bool
	}{
		{
			name: "A valid health check.",
			config: HealthCheckConfig{
				nodeID:              "mainnet",
				httpURL:             "http://rpc.ankr.io/eth",
				websocketURL:        "wss://something/something",
				useWsForBlockHeight: false,
			},
			isValid: true,
		},
		{
			name: "Healthcheck without httpURL.",
			config: HealthCheckConfig{
				nodeID:              "mainnet",
				websocketURL:        "wss://something/something",
				useWsForBlockHeight: true,
			},
			isValid: false,
		},
		{
			name: "Healthcheck without websocketURL when useWsForBlockHeight: true.",
			config: HealthCheckConfig{
				nodeID:              "mainnet",
				httpURL:             "http://rpc.ankr.io/eth",
				useWsForBlockHeight: true,
			},
			isValid: false,
		},
	} {
		healthCheckManager := NewHealthCheckManager(nil)

		assert.Equal(t, testCase.isValid, testCase.config.isValid())

		if !testCase.isValid {
			assert.Panics(t, func() { healthCheckManager.StartHealthChecks([]HealthCheckConfig{testCase.config}) })
		}
	}
}

func TestHealthChecks(t *testing.T) {
	configs := []HealthCheckConfig{
		{
			nodeID:              "mainnet",
			httpURL:             "http://rpc.ankr.io/eth",
			websocketURL:        "wss://something/something",
			useWsForBlockHeight: false,
		},
	}

	ethereumClient := mocks.NewEthClient(t)
	mockEthClientGetter := func(url string) (EthClient, error) {
		return ethereumClient, nil
	}

	ethereumClient.Mock.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
	ethereumClient.Mock.On("PeerCount", mock.Anything).
		Return(uint64(200), nil).Once()
	ethereumClient.Mock.On("SyncProgress", mock.Anything).
		Return(&ethereum.SyncProgress{}, nil).Once()

	healthCheckManager := NewHealthCheckManager(mockEthClientGetter)
	healthCheckManager.StartHealthChecks(configs)

	assert.Eventually(t, func() bool {
		return healthCheckManager.nodeIDToStatus["mainnet"].currentBlockNumber == uint64(100) &&
			healthCheckManager.nodeIDToStatus["mainnet"].peerCount == uint64(200) &&
			healthCheckManager.nodeIDToStatus["mainnet"].isSyncing
	}, 2*time.Second, 10*time.Millisecond, "NodeIDToStatus did not contain expected values")

	// Verify that NodeStatus is updated when the JSON RPC returns new values.
	ethereumClient.Mock.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return(&types.Header{Number: big.NewInt(1000)}, nil)
	ethereumClient.Mock.On("PeerCount", mock.Anything).
		Return(uint64(2000), nil)
	ethereumClient.Mock.On("SyncProgress", mock.Anything).
		Return(nil, nil)

	assert.Eventually(t, func() bool {
		return healthCheckManager.nodeIDToStatus["mainnet"].currentBlockNumber == uint64(1000) &&
			healthCheckManager.nodeIDToStatus["mainnet"].peerCount == uint64(2000) &&
			!healthCheckManager.nodeIDToStatus["mainnet"].isSyncing
	}, 2*time.Second, 10*time.Millisecond, "NodeIDToStatus did not contain expected values")
}
