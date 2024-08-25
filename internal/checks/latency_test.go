package checks

import (
	"testing"
	"time"

	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

func helperTestLatencyChecker(t *testing.T, latency1, latency2 time.Duration, isPassing bool) {
	t.Helper()
	ethClient := mocks.NewEthClient(t)
	ethClient.EXPECT().Latency(mock.Anything, "eth_call").Return(latency1, nil)
	ethClient.EXPECT().Latency(mock.Anything, "eth_getLogs").Return(latency2, nil)

	mockEthClientGetter := func(url string, credentials *config.BasicAuthConfig, additionalRequestHeaders *[]config.RequestHeaderConfig) (client.EthClient, error) { //nolint:nolintlint,revive // Legacy
		return ethClient, nil
	}

	checker := NewLatencyChecker(
		defaultUpstreamConfig,
		defaultRoutingConfig,
		mockEthClientGetter,
		metrics.NewContainer(config.TestChainName),
		zap.L(),
	)

	if isPassing {
		assert.True(t, checker.IsPassing())
	} else {
		assert.False(t, checker.IsPassing())
	}

	ethClient.AssertNumberOfCalls(t, "Latency", 2)
}

func TestLatencyChecker_TwoMethods_BothLatenciesLessThanThreshold(t *testing.T) {
	helperTestLatencyChecker(t, 2*time.Millisecond, 3*time.Millisecond, true)
}

func TestLatencyChecker_TwoMethods_BothLatenciesEqualToThreshold(t *testing.T) {
	helperTestLatencyChecker(t, 10000*time.Millisecond, 2000*time.Millisecond, true)
}

func TestLatencyChecker_TwoMethods_FirstLatencyTooHigh(t *testing.T) {
	helperTestLatencyChecker(t, 10001*time.Millisecond, 2000*time.Millisecond, false)
}

func TestLatencyChecker_TwoMethods_SecondLatencyTooHigh(t *testing.T) {
	helperTestLatencyChecker(t, 10000*time.Millisecond, 2002*time.Millisecond, false)
}

func TestLatencyChecker_TwoMethods_BothLatenciesTooHigh(t *testing.T) {
	helperTestLatencyChecker(t, 10002*time.Millisecond, 2003*time.Millisecond, false)
}
