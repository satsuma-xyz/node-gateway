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

	type testArgs struct {
		name string
		args args
		want RequestMetadata
	}

	testForMethod := func(methodName string, isStateRequired, isTraceMethod bool) testArgs {
		return testArgs{
			methodName,
			args{jsonrpc.RequestBody{Method: methodName}},
			RequestMetadata{
				IsStateRequired: isStateRequired,
				IsTraceMethod:   isTraceMethod,
			},
		}
	}

	tests := []testArgs{
		testForMethod("eth_call", true, false),
		testForMethod("eth_getBalance", true, false),
		testForMethod("eth_getBlockByNumber", false, false),
		testForMethod("eth_getTransactionReceipt", false, false),
		testForMethod("trace_filter", false, true),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RequestMetadataParser{}
			assert.Equalf(t, tt.want, p.Parse(tt.args.requestBody), "Parse(%v)", tt.args.requestBody)
		})
	}
}
