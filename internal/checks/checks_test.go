package checks

import (
	"errors"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/stretchr/testify/assert"
)

var defaultUpstreamConfig = &config.UpstreamConfig{
	ID:      "eth_mainnet",
	HTTPURL: "http://alchemy",
	WSURL:   "wss://alchemy",
}

type methodNotSupportedError struct{}

func (e methodNotSupportedError) Error() string  { return "GetMethod Not Supported." }
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
