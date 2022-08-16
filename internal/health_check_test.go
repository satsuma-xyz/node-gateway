package internal

import (
	"errors"
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
			ID:                "mainnet",
			HTTPURL:           "http://rpc.ankr.io/eth",
			WSURL:             "wss://something/something",
			HealthCheckConfig: HealthCheckConfig{UseWSForBlockHeight: newBool(false)},
		},
	}

	ethereumClient := mocks.NewEthClient(t)
	mockEthClientGetter := func(url string) (EthClient, error) {
		return ethereumClient, nil
	}

	ethereumClient.Mock.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return(&types.Header{Number: big.NewInt(100)}, nil).Once()
	ethereumClient.Mock.On("PeerCount", mock.Anything).
		Return(uint64(0), errors.New("an error")).Once()
	ethereumClient.Mock.On("SyncProgress", mock.Anything).
		Return(&ethereum.SyncProgress{}, nil).Once()

	healthCheckManager := NewHealthCheckManager(mockEthClientGetter, configs)
	healthCheckManager.StartHealthChecks()

	assert.Eventually(t, func() bool {
		return healthCheckManager.upstreamIDToStatus["mainnet"].currentBlockNumber == uint64(100) &&
			healthCheckManager.upstreamIDToStatus["mainnet"].peerCount == uint64(0) &&
			healthCheckManager.upstreamIDToStatus["mainnet"].peerCountError != nil &&
			healthCheckManager.upstreamIDToStatus["mainnet"].isSyncing
	}, 2*time.Second, 10*time.Millisecond, "UpstreamIDToStatus did not contain expected values")

	// Verify that UpstreamStatus is updated when the JSON RPC returns new values.
	ethereumClient.Mock.On("HeaderByNumber", mock.Anything, mock.Anything).
		Return(&types.Header{Number: big.NewInt(1000)}, nil)
	ethereumClient.Mock.On("PeerCount", mock.Anything).
		Return(uint64(2000), nil)
	ethereumClient.Mock.On("SyncProgress", mock.Anything).
		Return(nil, nil)

	assert.Eventually(t, func() bool {
		return healthCheckManager.upstreamIDToStatus["mainnet"].currentBlockNumber == uint64(1000) &&
			healthCheckManager.upstreamIDToStatus["mainnet"].peerCount == uint64(2000) &&
			healthCheckManager.upstreamIDToStatus["mainnet"].peerCountError == nil &&
			!healthCheckManager.upstreamIDToStatus["mainnet"].isSyncing
	}, 2*time.Second, 10*time.Millisecond, fmt.Sprintf("UpstreamIDToStatus did not contain expected values, actual: %+v", healthCheckManager.upstreamIDToStatus["mainnet"]))
}

type rpcError struct{}

func (e rpcError) Error() string  { return "Some RPC error" }
func (e rpcError) ErrorCode() int { return -3200 }

type methodNotSupportedError struct{}

func (e methodNotSupportedError) Error() string  { return "Method Not Supported." }
func (e methodNotSupportedError) ErrorCode() int { return JSONRPCErrCodeMethodNotFound }

func TestNodeStatus(t *testing.T) {
	for _, testCase := range []struct {
		upstreamStatus *UpstreamStatus
		name           string
		maxBlockHeight uint64
		healthy        bool
	}{
		{
			name: "A healthy upstream node with no errors and passing all checks.",
			upstreamStatus: &UpstreamStatus{
				currentBlockNumberError: nil,
				peerCountError:          nil,
				isSyncingError:          nil,
				connectionError:         nil,
				currentBlockNumber:      20,
				peerCount:               5,
				isSyncing:               false,
			},
			maxBlockHeight: 20,
			healthy:        true,
		},
		{
			name: "A unhealthy upstream node due to errors in healthchecking.",
			upstreamStatus: &UpstreamStatus{
				currentBlockNumberError: rpcError{},
				peerCountError:          nil,
				isSyncingError:          nil,
				connectionError:         nil,
				currentBlockNumber:      20,
				peerCount:               5,
				isSyncing:               false,
			},
			maxBlockHeight: 20,
			healthy:        false,
		},
		{
			name: "A healthy upstream node with that got 'method not supported errors' in healthchecks",
			upstreamStatus: &UpstreamStatus{
				currentBlockNumberError: nil,
				peerCountError:          methodNotSupportedError{},
				isSyncingError:          nil,
				connectionError:         nil,
				currentBlockNumber:      20,
				peerCount:               5,
				isSyncing:               false,
			},
			maxBlockHeight: 20,
			healthy:        true,
		},
		{
			name: "An unhealthy upstream node with less than max block height.",
			upstreamStatus: &UpstreamStatus{
				currentBlockNumberError: nil,
				peerCountError:          nil,
				isSyncingError:          nil,
				connectionError:         nil,
				currentBlockNumber:      19,
				peerCount:               5,
				isSyncing:               false,
			},
			maxBlockHeight: 20,
			healthy:        false,
		},
		{
			name: "An unhealthy upstream node with less than minimum peer count.",
			upstreamStatus: &UpstreamStatus{
				currentBlockNumberError: nil,
				peerCountError:          nil,
				isSyncingError:          nil,
				connectionError:         nil,
				currentBlockNumber:      20,
				peerCount:               4,
				isSyncing:               false,
			},
			maxBlockHeight: 20,
			healthy:        false,
		},
		{
			name: "An unhealthy upstream node that is still syncing.",
			upstreamStatus: &UpstreamStatus{
				currentBlockNumberError: nil,
				peerCountError:          nil,
				isSyncingError:          nil,
				connectionError:         nil,
				currentBlockNumber:      20,
				peerCount:               5,
				isSyncing:               true,
			},
			maxBlockHeight: 20,
			healthy:        false,
		}} {
		if testCase.healthy {
			assert.True(t, testCase.upstreamStatus.isHealthy(testCase.maxBlockHeight), fmt.Sprintf("Test case: %s failed", testCase.name))
		} else {
			assert.False(t, testCase.upstreamStatus.isHealthy(testCase.maxBlockHeight), fmt.Sprintf("Test case: %s failed", testCase.name))
		}
	}
}
