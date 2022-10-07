package metadata

import (
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

type RequestMetadataParser struct{}

func (p *RequestMetadataParser) Parse(batchRequest jsonrpc.BatchRequestBody) RequestMetadata {
	result := RequestMetadata{
		IsStateRequired: false,
	}

	for _, requestBody := range batchRequest.Requests {
		switch requestBody.Method {
		case "eth_getBalance", "eth_getStorageAt", "eth_getTransactionCount", "eth_getCode", "eth_call", "eth_estimateGas":
			// List of state methods: https://ethereum.org/en/developers/docs/apis/json-rpc/#state_methods
			result.IsStateRequired = true
		}
	}

	return result
}
