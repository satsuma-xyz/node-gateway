package metadata

import (
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

type RequestMetadataParser struct{}

func (p *RequestMetadataParser) Parse(requestBody jsonrpc.RequestBody) RequestMetadata {
	switch requestBody.(type) {
	case *jsonrpc.SingleRequestBody:
		return RequestMetadata{
			IsStateRequired: isStateRequiredForMethod(requestBody.GetMethod()),
			IsTraceMethod:   isTraceMethod(requestBody.GetMethod()),
		}
	case *jsonrpc.BatchRequestBody:
		result := RequestMetadata{
			IsStateRequired: false,
			IsTraceMethod:   false,
		}

		for _, requestBody := range requestBody.GetSubRequests() {
			if isStateRequiredForMethod(requestBody.Method) {
				result.IsStateRequired = true
			}

			if isTraceMethod(requestBody.Method) {
				result.IsTraceMethod = true
			}

			if result.IsStateRequired && result.IsTraceMethod {
				break
			}
		}

		return result
	default:
		panic("Invalid request body type   ")
	}
}

func isStateRequiredForMethod(method string) bool {
	switch method {
	case "eth_getBalance", "eth_getStorageAt", "eth_getTransactionCount", "eth_getCode", "eth_call", "eth_estimateGas":
		// List of state methods: https://ethereum.org/en/developers/docs/apis/json-rpc/#state_methods
		return true
	default:
		return false
	}
}

func isTraceMethod(method string) bool {
	switch method {
	case "trace_filter", "trace_block", "trace_get", "trace_transaction", "trace_call", "trace_callMany",
		"trace_rawTransaction", "trace_replayBlockTransactions", "trace_replayTransaction":
		// List of trace methods: https://openethereum.github.io/JSONRPC-trace-module
		return true
	default:
		return false
	}

}
