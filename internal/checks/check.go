package checks

import (
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
)

const (
	JSONRPCErrCodeMethodNotFound = -32601
	MinimumPeerCount             = 5
)

func isMethodNotSupportedErr(err error) bool {
	if err == nil {
		return false
	}

	switch e := err.(type) {
	case rpc.Error:
		return e.ErrorCode() == JSONRPCErrCodeMethodNotFound
	default:
		// Some node providers like Alchemy do not follow the JSON-RPC spec and
		// return errors that don't use JSONRPCErrCodeMethodNotFound and don't
		// match rpc.Error.
		return strings.Contains(strings.ToLower(err.Error()), "unsupported method")
	}
}

//go:generate mockery --output ../mocks --name Checker
type Checker interface {
	RunCheck()
	IsPassing() bool
}
