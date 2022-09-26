package checks

import (
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/prometheus/client_golang/prometheus"
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

func runCheckWithMetrics(runCheck func(), counterMetrics prometheus.Counter, durationMetrics prometheus.Observer) {
	start := time.Now()

	counterMetrics.Inc()
	runCheck()
	durationMetrics.Observe(time.Since(start).Seconds())
}
