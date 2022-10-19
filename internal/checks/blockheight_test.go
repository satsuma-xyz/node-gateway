package checks

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/metrics"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockSubscription struct{}

func (m *mockSubscription) Unsubscribe() {}

func (m *mockSubscription) Err() <-chan error { return make(chan error) }

const maxBlockHeight = 50000

func TestBlockHeightChecker_WS(t *testing.T) {
	ethClient := mocks.NewEthClient(t)
	ethClient.On("SubscribeNewHead", mock.Anything, mock.Anything).Return(&mockSubscription{}, nil)
	ethClient.On("HeaderByNumber", mock.Anything, mock.Anything).Return(&types.Header{Number: big.NewInt(int64(maxBlockHeight))}, nil)

	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethClient, nil
	}

	chainMetadataStore := metadata.NewChainMetadataStore()
	chainMetadataStore.Start()

	checker := NewBlockHeightChecker(defaultUpstreamConfig, mockEthClientGetter, chainMetadataStore, metrics.NewContainer())

	ethClient.AssertNumberOfCalls(t, "SubscribeNewHead", 1)

	checker.RunCheck()
	ethClient.AssertNumberOfCalls(t, "HeaderByNumber", 0)

	// Websockets encounters an error. Now RunCheck should use HTTP.
	checker.(*BlockHeightCheck).webSocketError = errors.New("some error")
	assert.False(t, checker.IsPassing(maxBlockHeight))

	checker.RunCheck()
	ethClient.AssertNumberOfCalls(t, "HeaderByNumber", 1)
	assert.True(t, checker.IsPassing(maxBlockHeight))
}

func TestBlockHeightChecker_WSSubscribeFailed(t *testing.T) {
	ethClient := mocks.NewEthClient(t)
	ethClient.On("SubscribeNewHead", mock.Anything, mock.Anything).Return(nil, errors.New("some error"))
	ethClient.On("HeaderByNumber", mock.Anything, mock.Anything).Return(&types.Header{Number: big.NewInt(int64(50000))}, nil)

	mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
		return ethClient, nil
	}

	chainMetadataStore := metadata.NewChainMetadataStore()
	chainMetadataStore.Start()

	checker := NewBlockHeightChecker(defaultUpstreamConfig, mockEthClientGetter, chainMetadataStore, metrics.NewContainer())

	ethClient.AssertNumberOfCalls(t, "SubscribeNewHead", 1)
	assert.False(t, checker.IsPassing(maxBlockHeight))

	checker.RunCheck()
	ethClient.AssertNumberOfCalls(t, "HeaderByNumber", 1)
	assert.True(t, checker.IsPassing(maxBlockHeight))
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
		ethClient.On("HeaderByNumber", mock.Anything, mock.Anything).Return(&types.Header{Number: big.NewInt(int64(maxBlockHeight))}, nil)

		mockEthClientGetter := func(url string, credentials *client.BasicAuthCredentials) (client.EthClient, error) {
			return ethClient, nil
		}

		chainMetadataStore := metadata.NewChainMetadataStore()
		chainMetadataStore.Start()

		checker := NewBlockHeightChecker(config, mockEthClientGetter, chainMetadataStore, metrics.NewContainer())

		checker.RunCheck()
		ethClient.AssertNumberOfCalls(t, "SubscribeNewHead", 0)
		ethClient.AssertNumberOfCalls(t, "HeaderByNumber", 1)
		assert.True(t, checker.IsPassing(maxBlockHeight))
	}
}

func TestBlockHeightChecker_IsPassing(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		blockHeightCheck BlockHeightCheck
		blockHeight      uint64
		isPassing        bool
	}{
		{
			name: "No errors, block height high enough.",
			blockHeightCheck: BlockHeightCheck{
				upstreamConfig: defaultUpstreamConfig,
				blockHeight:    3,
			},
			blockHeight: 3,
			isPassing:   true,
		},
		{
			name: "No errors, block height too low.",
			blockHeightCheck: BlockHeightCheck{
				upstreamConfig: defaultUpstreamConfig,
				blockHeight:    2,
			},
			blockHeight: 3,
			isPassing:   false,
		},
		{
			name: "Errors found, block height high enough.",
			blockHeightCheck: BlockHeightCheck{
				blockHeightError: errors.New("an error"),
				upstreamConfig:   defaultUpstreamConfig,
				blockHeight:      3,
			},
			blockHeight: 3,
			isPassing:   false,
		},
	} {
		if testCase.isPassing {
			assert.True(t, testCase.blockHeightCheck.IsPassing(testCase.blockHeight), fmt.Sprintf("Test case %s failed", testCase.name))
		} else {
			assert.False(t, testCase.blockHeightCheck.IsPassing(testCase.blockHeight), fmt.Sprintf("Test case %s failed", testCase.name))
		}
	}
}
