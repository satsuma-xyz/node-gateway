package metadata

import (
	"testing"

	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/stretchr/testify/assert"
)

func TestRequestMetadataParser_Parse(t *testing.T) {
	type args struct {
		requestBody jsonrpc.BatchRequestBody
	}

	type testArgs struct {
		name string
		args args
		want RequestMetadata
	}

	testForMethod := func(methodName string, isStateRequired bool) testArgs {
		return testArgs{
			methodName,
			args{jsonrpc.BatchRequestBody{
				Requests: []jsonrpc.RequestBody{{Method: methodName}},
			}},
			RequestMetadata{IsStateRequired: isStateRequired},
		}
	}

	tests := []testArgs{
		testForMethod("eth_call", true),
		testForMethod("eth_getBalance", true),
		testForMethod("eth_getBlockByNumber", false),
		testForMethod("eth_getTransactionReceipt", false),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RequestMetadataParser{}
			assert.Equalf(t, tt.want, p.Parse(tt.args.requestBody), "Parse(%v)", tt.args.requestBody)
		})
	}
}
