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
	ethClient.EXPECT().RecordLatency(mock.Anything, "eth_call").Return(latency1, nil)
	ethClient.EXPECT().RecordLatency(mock.Anything, "eth_getLogs").Return(latency2, nil)

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

	ethClient.AssertNumberOfCalls(t, "RecordLatency", 2)
}

func TestLatencyChecker_TwoMethods_BothLatenciesLessThanThreshold(t *testing.T) {
	helperTestLatencyChecker(t, 2*time.Millisecond, 3*time.Millisecond, true)
}

func TestLatencyChecker_TwoMethods_BothLatenciesEqualToThreshold(t *testing.T) {
	helperTestLatencyChecker(t, (10000-1)*time.Millisecond, (2000-1)*time.Millisecond, true)
}

func TestLatencyChecker_TwoMethods_FirstLatencyTooHigh(t *testing.T) {
	helperTestLatencyChecker(t, 10000*time.Millisecond, (2000-1)*time.Millisecond, false)
}

func TestLatencyChecker_TwoMethods_SecondLatencyTooHigh(t *testing.T) {
	helperTestLatencyChecker(t, (10000-1)*time.Millisecond, 2000*time.Millisecond, false)
}

func TestLatencyChecker_TwoMethods_BothLatenciesTooHigh(t *testing.T) {
	helperTestLatencyChecker(t, 10002*time.Millisecond, 2003*time.Millisecond, false)
}

func Test_isMatchForPatterns_True(t *testing.T) {
	Assert := assert.New(t)

	Assert.True(isMatchForPatterns("400", []string{}))

	Assert.True(isMatchForPatterns("400", []string{"400"}))
	Assert.True(isMatchForPatterns("400", []string{"4XX"}))
	Assert.True(isMatchForPatterns("400", []string{"X00"}))
	Assert.True(isMatchForPatterns("400", []string{"400", "500"}))
	Assert.True(isMatchForPatterns("500", []string{"400", "500"}))
}

func Test_isMatchForPatterns_False(t *testing.T) {
	Assert := assert.New(t)

	Assert.False(isMatchForPatterns("", []string{""}))
	Assert.False(isMatchForPatterns("", []string{"400"}))

	Assert.False(isMatchForPatterns("400", []string{"500"}))
	Assert.False(isMatchForPatterns("400", []string{"4X1"}))
	Assert.False(isMatchForPatterns("410", []string{"X00"}))
	Assert.False(isMatchForPatterns("400", []string{"401", "5X0"}))
	Assert.False(isMatchForPatterns("503", []string{"400", "500"}))
}

func Test_isMatch_True(t *testing.T) {
	Assert := assert.New(t)

	Assert.True(isMatch("400", "400"))
	Assert.True(isMatch("400", "4x0"))
	Assert.True(isMatch("400", "40X"))
	Assert.True(isMatch("400", "4Xx"))
	Assert.True(isMatch("400", "XxX"))

	Assert.True(isMatch("38765", "38XXX"))
	Assert.True(isMatch("38765", "XX765"))
}

func Test_isMatch_False(t *testing.T) {
	Assert := assert.New(t)

	Assert.False(isMatch("400", "40"))
	Assert.False(isMatch("40", "400"))

	Assert.False(isMatch("400", "500"))
	Assert.False(isMatch("400", "4x2"))
	Assert.False(isMatch("400", "41X"))
	Assert.False(isMatch("400", "6Xx"))
	Assert.False(isMatch("400", "X7X"))
}

func Test_isErrorMatches_True(t *testing.T) {
	Assert := assert.New(t)

	Assert.True(isErrorMatches("a", []string{}))
	Assert.True(isErrorMatches("a", []string{"a"}))
	Assert.True(isErrorMatches("aa", []string{"a"}))
	Assert.True(isErrorMatches("a", []string{"a", "b"}))
	Assert.True(isErrorMatches("aa", []string{"a", "b"}))
	Assert.True(isErrorMatches("some error", []string{"a", "err"}))
	Assert.True(isErrorMatches("error string", []string{"error string"}))
	Assert.True(isErrorMatches("prefix error string suffix", []string{"error string"}))

	Assert.True(isErrorMatches("aba", []string{"ab"}))
	Assert.True(isErrorMatches("aba", []string{"ba"}))
}

func Test_isErrorMatches_False(t *testing.T) {
	Assert := assert.New(t)

	Assert.False(isErrorMatches("", []string{}))
	Assert.False(isErrorMatches("", []string{"a", "b"}))

	Assert.False(isErrorMatches("b", []string{"a"}))
	Assert.False(isErrorMatches("a", []string{"aa"}))
	Assert.False(isErrorMatches("aa", []string{"aba"}))
}
