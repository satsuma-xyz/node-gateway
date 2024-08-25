package checks

import (
	"errors"
	"testing"
	"time"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/stretchr/testify/assert"
)

var defaultUpstreamConfig = &config.UpstreamConfig{
	ID:      "eth_mainnet",
	HTTPURL: "http://alchemy",
	WSURL:   "wss://alchemy",
}

var defaultRoutingConfig = &config.RoutingConfig{
	DetectionWindow: config.NewDuration(10 * time.Minute),
	BanWindow:       config.NewDuration(50 * time.Minute),
	Errors: &config.ErrorsConfig{
		Rate: 0.25,
		HTTPCodes: []string{
			"5xx",
			"420",
		},
		JSONRPCCodes: []string{
			"32xxx",
		},
		ErrorStrings: []string{
			"internal server error",
		},
	},
	Latency: &config.LatencyConfig{
		MethodLatencyThresholds: map[string]time.Duration{
			"eth_call":    10000 * time.Millisecond,
			"eth_getLogs": 2000 * time.Millisecond,
		},
		Threshold: 1000 * time.Millisecond,
		Methods: []config.MethodConfig{
			{
				Name:      "eth_getLogs",
				Threshold: 2000 * time.Millisecond,
			},
			{
				Name:      "eth_call",
				Threshold: 10000 * time.Millisecond,
			},
		},
	},
}

type methodNotSupportedError struct{}

func (e methodNotSupportedError) Error() string  { return "Method Not Supported." }
func (e methodNotSupportedError) ErrorCode() int { return -32601 }

func TestMethodIsNotSupportedError(t *testing.T) {
	for _, testCase := range []struct {
		err                  error
		name                 string
		isMethodNotSupported bool
	}{
		{
			name:                 "Error with the unsupported method error code.",
			err:                  methodNotSupportedError{},
			isMethodNotSupported: true,
		},
		{
			name:                 "Error with unsupported method in error message.",
			err:                  errors.New("unsupported method"),
			isMethodNotSupported: true,
		},
		{
			name:                 "Some other error that is not considered an unsupported method error.",
			err:                  errors.New("something else"),
			isMethodNotSupported: false,
		},
	} {
		if testCase.isMethodNotSupported {
			assert.True(t, isMethodNotSupportedErr(testCase.err), testCase.name)
		} else {
			assert.False(t, isMethodNotSupportedErr(testCase.err), testCase.name)
		}
	}
}
