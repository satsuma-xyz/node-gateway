package metadata

import (
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

type RequestMetadataParser struct{}

func (p *RequestMetadataParser) Parse(batchRequest jsonrpc.RequestBody) RequestMetadata {
	switch batchRequest.(type) {
	case *jsonrpc.SingleRequestBody:
		return RequestMetadata{IsStateRequired: isStateRequiredForMethod(batchRequest.GetMethod())}
	case *jsonrpc.BatchRequestBody:
		result := RequestMetadata{IsStateRequired: false}

		br := batchRequest.(*jsonrpc.BatchRequestBody)
		for _, requestBody := range br.Requests {
			if isStateRequiredForMethod(requestBody.Method) {
				result.IsStateRequired = true
			}
		}

		return result
	default:
		panic("Unknown RequestBody type!")
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
