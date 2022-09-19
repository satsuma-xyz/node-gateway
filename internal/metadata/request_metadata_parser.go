package metadata

import (
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

type RequestMetadataParser struct{}

func (p *RequestMetadataParser) Parse(requestBody jsonrpc.RequestBody) RequestMetadata {
	result := RequestMetadata{}

	switch requestBody.Method {
	case "eth_getBalance", "eth_getStorageAt", "eth_getTransactionCount", "eth_getCode", "eth_call", "eth_estimateGas":
		result.IsStateRequired = true
	default:
		result.IsStateRequired = false
	}

	return result
}
