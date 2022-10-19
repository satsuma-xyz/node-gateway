package checks

import (
	"errors"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func TestPeerChecker(t *testing.T) {
	ethClient := mocks.NewEthClient(t)
	ethClient.EXPECT().PeerCount(mock.Anything).Return(uint64(6), nil)

	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethClient, nil
	}

	checker := NewPeerChecker(defaultUpstreamConfig, mockEthClientGetter, metrics.NewContainer(), zap.L())

	assert.True(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "PeerCount", 1)

	ethClient.ExpectedCalls = nil
	ethClient.On("PeerCount", mock.Anything).Return(uint64(4), nil)

	checker.RunCheck()
	assert.False(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "PeerCount", 2)

	ethClient.ExpectedCalls = nil
	ethClient.On("PeerCount", mock.Anything).Return(uint64(0), errors.New("some error"))

	checker.RunCheck()
	assert.False(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "PeerCount", 3)

	ethClient.ExpectedCalls = nil
	ethClient.On("PeerCount", mock.Anything).Return(uint64(6), nil)

	checker.RunCheck()
	assert.True(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "PeerCount", 4)
}

func TestPeerChecker_MethodNotSupported(t *testing.T) {
	ethClient := mocks.NewEthClient(t)
	ethClient.EXPECT().PeerCount(mock.Anything).Return(uint64(0), methodNotSupportedError{})

	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethClient, nil
	}

	checker := NewPeerChecker(defaultUpstreamConfig, mockEthClientGetter, metrics.NewContainer(), zap.L())

	assert.True(t, checker.IsPassing())
	ethClient.AssertNumberOfCalls(t, "PeerCount", 1)

	checker.RunCheck()
	ethClient.AssertNumberOfCalls(t, "PeerCount", 1)
}
