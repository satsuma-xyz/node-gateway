package metadata

import (
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

type RequestMetadataParser struct{}

func (p *RequestMetadataParser) Parse(requestBody jsonrpc.RequestBody) RequestMetadata {
	result := RequestMetadata{}

	switch requestBody.Method {
	case "eth_getBalance", "eth_getStorageAt", "eth_getTransactionCount", "eth_getCode", "eth_call", "eth_estimateGas":
		// List of state methods: https://ethereum.org/en/developers/docs/apis/json-rpc/#state_methods
		result.IsStateRequired = true
	case "trace_filter", "trace_block", "trace_get", "trace_transaction", "trace_call", "trace_callMany",
		"trace_rawTransaction", "trace_replayBlockTransactions", "trace_replayTransaction":
		// List of trace methods: https://openethereum.github.io/JSONRPC-trace-module
		result.IsTraceMethod = true
	default:
		result.IsStateRequired = false
	}

	return result
}
