package internal

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHealthChecks(t *testing.T) {
	configs := []UpstreamConfig{
		{
			ID:                  "mainnet",
			HTTPURL:             "http://rpc.ankr.io/eth",
			WSURL:               "wss://something/something",
			UseWsForBlockHeight: false,
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
