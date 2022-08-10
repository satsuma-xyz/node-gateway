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

func TestHealthCheckConfig_Valid(t *testing.T) {
	config := HealthCheckConfig{
		nodeID:                  "mainnet",
		httpURL:                 "http://rpc.ankr.io/eth",
		websocketURL:            "wss://something/something",
		shouldSubscribeNewHeads: false,
	}
	assert.True(t, config.validate())
}

func TestHealthCheckConfig_InvalidNoHttpUrl(t *testing.T) {
	config := HealthCheckConfig{
		nodeID:                  "mainnet",
		websocketURL:            "wss://something/something",
		shouldSubscribeNewHeads: true,
	}
	assert.False(t, config.validate())
	assert.Panics(t, func() { StartHealthChecks([]HealthCheckConfig{config}) })
}

func TestHealthCheckConfig_InvalidWebsockets(t *testing.T) {
	config := HealthCheckConfig{
		nodeID:                  "mainnet",
		httpURL:                 "http://rpc.ankr.io/eth",
		shouldSubscribeNewHeads: true,
	}
	assert.False(t, config.validate())
	assert.Panics(t, func() { StartHealthChecks([]HealthCheckConfig{config}) })
}

func TestHealthChecks(t *testing.T) {
	configs := []HealthCheckConfig{
		{
			nodeID:                  "mainnet",
			httpURL:                 "http://rpc.ankr.io/eth",
			websocketURL:            "wss://something/something",
			shouldSubscribeNewHeads: false,
		},
	}

	ethereumClient := mocks.NewEthClient(t)
	getEthereumClient = func(url string) (EthClient, error) {
		return ethereumClient, nil
	}

	ethereumClient.Mock.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
	ethereumClient.Mock.On("PeerCount", mock.Anything).
		Return(uint64(200), nil).Once()
	ethereumClient.Mock.On("SyncProgress", mock.Anything).
		Return(&ethereum.SyncProgress{}, nil).Once()

	StartHealthChecks(configs)

	assert.Eventually(t, func() bool {
		return NodeIDToStatus["mainnet"].currentBlockNumber == uint64(100) &&
			NodeIDToStatus["mainnet"].peerCount == uint64(200) &&
			NodeIDToStatus["mainnet"].isSyncing
	}, 2*time.Second, 10*time.Millisecond, "NodeIDToStatus did contain expected values")

	// Nodes changed
	ethereumClient.Mock.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return(&types.Header{Number: big.NewInt(1000)}, nil)
	ethereumClient.Mock.On("PeerCount", mock.Anything).
		Return(uint64(2000), nil)
	ethereumClient.Mock.On("SyncProgress", mock.Anything).
		Return(nil, nil)

	assert.Eventually(t, func() bool {
		return NodeIDToStatus["mainnet"].currentBlockNumber == uint64(1000) &&
			NodeIDToStatus["mainnet"].peerCount == uint64(2000) &&
			!NodeIDToStatus["mainnet"].isSyncing
	}, 2*time.Second, 10*time.Millisecond, "NodeIDToStatus did contain expected values")
}
