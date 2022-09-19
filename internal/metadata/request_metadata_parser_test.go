package metadata

import (
	"testing"

	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/stretchr/testify/assert"
)

func TestRequestMetadataParser_Parse(t *testing.T) {
	type args struct {
		requestBody jsonrpc.RequestBody
	}

	tests := []struct {
		name string
		args args
		want RequestMetadata
	}{
		{"eth_call", args{jsonrpc.RequestBody{Method: "eth_call"}}, RequestMetadata{IsStateRequired: true}},
		{"eth_getBalance", args{jsonrpc.RequestBody{Method: "eth_getBalance"}}, RequestMetadata{IsStateRequired: true}},
		{"eth_getBlockByNumber", args{jsonrpc.RequestBody{Method: "eth_getBlockByNumber"}}, RequestMetadata{IsStateRequired: false}},
		{"eth_getTransactionReceipt", args{jsonrpc.RequestBody{Method: "eth_getTransactionReceipt"}}, RequestMetadata{IsStateRequired: false}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RequestMetadataParser{}
			assert.Equalf(t, tt.want, p.Parse(tt.args.requestBody), "Parse(%v)", tt.args.requestBody)
		})
	}
}
