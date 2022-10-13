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
		args args
		name string
		want RequestMetadata
	}

	testForSingleRequest := func(methodName string, isStateRequired, isTraceMethod bool) testArgs {
		return testArgs{
			args{
				requestBody: &jsonrpc.SingleRequestBody{
					Method: methodName,
				},
			},
			methodName,
			RequestMetadata{
				IsStateRequired: isStateRequired,
				IsTraceMethod:   isTraceMethod,
			},
		}
	}

	testForBatchRequest := func(testName string, methodNames []string, isStateRequired bool, isTraceMethod bool) testArgs {
		requests := make([]jsonrpc.SingleRequestBody, 0)
		for _, methodName := range methodNames {
			requests = append(requests, jsonrpc.SingleRequestBody{
				Method: methodName,
			})
		}

		return testArgs{
			args{
				requestBody: &jsonrpc.BatchRequestBody{
					Requests: requests,
				},
			},
			testName,
			RequestMetadata{
				IsStateRequired: isStateRequired,
				IsTraceMethod:   isTraceMethod,
			},
		}
	}

	tests := []testArgs{
		testForSingleRequest("eth_call", true, false),
		testForSingleRequest("eth_getBalance", true, false),
		testForSingleRequest("eth_getBlockByNumber", false, false),
		testForSingleRequest("eth_getTransactionReceipt", false, false),
		testForSingleRequest("trace_filter", false, true),
		testForSingleRequest("trace_replayBlockTransactions", false, true),
		testForBatchRequest("batch eth_call", []string{"eth_call"}, true, false),
		testForBatchRequest("batch eth_call w/ eth_getTransactionReceipt",
			[]string{"eth_call", "eth_getTransactionReceipt"}, true, false),
		testForBatchRequest("batch trace w/ eth_getTransactionReceipt",
			[]string{"trace_filter", "eth_getTransactionReceipt"}, false, true),
		testForBatchRequest("batch eth_call w/ eth_getTransactionReceipt and trace",
			[]string{"eth_call", "eth_getTransactionReceipt", "trace_filter"}, true, true),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RequestMetadataParser{}
			assert.Equalf(t, tt.want, p.Parse(tt.args.requestBody), "Parse(%v)", tt.args.requestBody)
		})
	}
}
