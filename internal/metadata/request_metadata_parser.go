package metadata

import (
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

type RequestMetadataParser struct{}

func (p *RequestMetadataParser) Parse(requestBody jsonrpc.RequestBody) RequestMetadata {
	switch requestBody.(type) {
	case *jsonrpc.SingleRequestBody:
		return RequestMetadata{
			Methods: []string{requestBody.GetMethod()},
		}
	case *jsonrpc.BatchRequestBody:
		result := RequestMetadata{
			Methods: []string{},
		}

		for _, requestBody := range requestBody.GetSubRequests() {
			result.Methods = append(result.Methods, requestBody.Method)
		}

		return result
	default:
		panic("Invalid request body type   ")
	}
}
