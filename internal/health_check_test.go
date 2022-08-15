package internal

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHealthCheckManager(t *testing.T) {
	configs := []UpstreamConfig{
		{
			ID:      "mainnet",
			HTTPURL: "http://rpc.ankr.io/eth",
			WSURL:   "wss://something/something",
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
	}, 2*time.Second, 10*time.Millisecond, fmt.Sprintf("NodeIDToStatus did not contain expected values, actual: %+v", healthCheckManager.nodeIDToStatus["mainnet"]))
}

type rpcError struct{}

func (e rpcError) Error() string  { return "Some RPC error" }
func (e rpcError) ErrorCode() int { return -3200 }

type methodNotSupportedError struct{}

func (e methodNotSupportedError) Error() string  { return "Method Not Supported." }
func (e methodNotSupportedError) ErrorCode() int { return JSONRPCErrCodeMethodNotFound }

func TestNodeStatus(t *testing.T) {
	for _, testCase := range []struct {
		nodeStatus  *NodeStatus
		globalState *globalState
		name        string
		healthy     bool
	}{
		{
			name: "A healthy node with no errors and passing all checks.",
			nodeStatus: &NodeStatus{
				getCurrentBlockNumberError: healthCheckError{},
				getPeerCountError:          healthCheckError{},
				getIsSyncingError:          healthCheckError{},
				connectionError:            healthCheckError{},
				currentBlockNumber:         20,
				peerCount:                  5,
				isSyncing:                  false,
			},
			globalState: &globalState{
				maxBlockHeight: 20,
			},
			healthy: true,
		},
		{
			name: "A unhealthy node due to errors in healthchecking.",
			nodeStatus: &NodeStatus{
				getCurrentBlockNumberError: healthCheckError{err: rpcError{}},
				getPeerCountError:          healthCheckError{},
				getIsSyncingError:          healthCheckError{},
				connectionError:            healthCheckError{},
				currentBlockNumber:         20,
				peerCount:                  5,
				isSyncing:                  false,
			},
			globalState: &globalState{
				maxBlockHeight: 20,
			},
			healthy: false,
		},
		{
			name: "A healthy node with that got 'method not supported errors' in healthchecks",
			nodeStatus: &NodeStatus{
				getCurrentBlockNumberError: healthCheckError{},
				getPeerCountError:          healthCheckError{err: methodNotSupportedError{}},
				getIsSyncingError:          healthCheckError{},
				connectionError:            healthCheckError{},
				currentBlockNumber:         20,
				peerCount:                  5,
				isSyncing:                  false,
			},
			globalState: &globalState{
				maxBlockHeight: 20,
			},
			healthy: true,
		},
		{
			name: "An unhealthy node with less than max block height.",
			nodeStatus: &NodeStatus{
				getCurrentBlockNumberError: healthCheckError{},
				getPeerCountError:          healthCheckError{},
				getIsSyncingError:          healthCheckError{},
				connectionError:            healthCheckError{},
				currentBlockNumber:         19,
				peerCount:                  5,
				isSyncing:                  false,
			},
			globalState: &globalState{
				maxBlockHeight: 20,
			},
			healthy: false,
		},
		{
			name: "An unhealthy node with less than minimum peer count.",
			nodeStatus: &NodeStatus{
				getCurrentBlockNumberError: healthCheckError{},
				getPeerCountError:          healthCheckError{},
				getIsSyncingError:          healthCheckError{},
				connectionError:            healthCheckError{},
				currentBlockNumber:         20,
				peerCount:                  4,
				isSyncing:                  false,
			},
			globalState: &globalState{
				maxBlockHeight: 20,
			},
			healthy: false,
		},
		{
			name: "An unhealthy node that is still syncing.",
			nodeStatus: &NodeStatus{
				getCurrentBlockNumberError: healthCheckError{},
				getPeerCountError:          healthCheckError{},
				getIsSyncingError:          healthCheckError{},
				connectionError:            healthCheckError{},
				currentBlockNumber:         20,
				peerCount:                  5,
				isSyncing:                  true,
			},
			globalState: &globalState{
				maxBlockHeight: 20,
			},
			healthy: false,
		}} {
		if testCase.healthy {
			assert.True(t, testCase.nodeStatus.isHealthy(testCase.globalState), fmt.Sprintf("Test case: %s failed", testCase.name))
		} else {
			assert.False(t, testCase.nodeStatus.isHealthy(testCase.globalState), fmt.Sprintf("Test case: %s failed", testCase.name))
		}
	}
}
