package checks

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSyncingChecker(t *testing.T) {
	ethClient := mocks.NewEthClient(t)
	ethClient.On("SyncProgress", mock.Anything).Return(&ethereum.SyncProgress{}, nil)

	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethClient, nil
	}

	checker := NewSyncingChecker(defaultUpstreamConfig, mockEthClientGetter, metrics.NewContainer())

	assert.False(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "SyncProgress", 1)

	ethClient.ExpectedCalls = nil
	ethClient.On("SyncProgress", mock.Anything).Return(nil, nil)

	checker.RunCheck()
	assert.True(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "SyncProgress", 2)

	ethClient.ExpectedCalls = nil
	ethClient.On("SyncProgress", mock.Anything).Return(nil, errors.New("some error"))

	checker.RunCheck()
	assert.False(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "SyncProgress", 3)
}

func TestSyncingChecker_MethodNotSupported(t *testing.T) {
	ethClient := mocks.NewEthClient(t)
	ethClient.On("SyncProgress", mock.Anything).Return(nil, methodNotSupportedError{})

	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethClient, nil
	}

	checker := NewSyncingChecker(defaultUpstreamConfig, mockEthClientGetter, metrics.NewContainer())

	assert.True(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "SyncProgress", 1)

	checker.RunCheck()
	ethClient.AssertNumberOfCalls(t, "SyncProgress", 1)
}
