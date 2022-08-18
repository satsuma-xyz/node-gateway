package checks

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/mock"
)

type mockSubscription struct{}

func (m *mockSubscription) Unsubscribe() {}

func (m *mockSubscription) Err() <-chan error { return make(chan error) }

func TestBlockHeightChecker_WS(t *testing.T) {
	ethClient := mocks.NewEthClient(t)
	ethClient.On("SubscribeNewHead", mock.Anything, mock.Anything).Return(&mockSubscription{}, nil)
	ethClient.On("HeaderByNumber", mock.Anything, mock.Anything).Return(&types.Header{Number: big.NewInt(int64(50000))}, nil)

	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethClient, nil
	}

	checker := NewBlockHeightChecker(defaultUpstreamConfig, mockEthClientGetter)

	ethClient.AssertNumberOfCalls(t, "SubscribeNewHead", 1)

	checker.RunCheck()
	ethClient.AssertNumberOfCalls(t, "HeaderByNumber", 0)

	// Websockets encounters an error. Now RunCheck should use HTTP.
	checker.(*BlockHeightCheck).WebSocketError = errors.New("some error")

	checker.RunCheck()
	ethClient.AssertNumberOfCalls(t, "HeaderByNumber", 1)
}

func TestBlockHeightChecker_WSSubscribeFailed(t *testing.T) {
	ethClient := mocks.NewEthClient(t)
	ethClient.On("SubscribeNewHead", mock.Anything, mock.Anything).Return(nil, errors.New("some error"))
	ethClient.On("HeaderByNumber", mock.Anything, mock.Anything).Return(&types.Header{Number: big.NewInt(int64(50000))}, nil)

	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethClient, nil
	}

	checker := NewBlockHeightChecker(defaultUpstreamConfig, mockEthClientGetter)

	ethClient.AssertNumberOfCalls(t, "SubscribeNewHead", 1)

	checker.RunCheck()
	ethClient.AssertNumberOfCalls(t, "HeaderByNumber", 1)
}

func TestBlockHeightChecker_HTTP(t *testing.T) {
	for _, config := range []*config.UpstreamConfig{
		{
			ID:      "eth_mainnet",
			HTTPURL: "http://alchemy",
		},
		{
			ID:      "eth_mainnet",
			HTTPURL: "http://alchemy",
			WSURL:   "wss://alchemy",
			HealthCheckConfig: config.HealthCheckConfig{
				UseWSForBlockHeight: new(bool),
			},
		},
	} {
		ethClient := mocks.NewEthClient(t)
		ethClient.On("HeaderByNumber", mock.Anything, mock.Anything).Return(&types.Header{Number: big.NewInt(int64(50000))}, nil)

		mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
			return ethClient, nil
		}

		checker := NewBlockHeightChecker(config, mockEthClientGetter)

		checker.RunCheck()
		ethClient.AssertNumberOfCalls(t, "SubscribeNewHead", 0)
		ethClient.AssertNumberOfCalls(t, "HeaderByNumber", 1)
	}
}
