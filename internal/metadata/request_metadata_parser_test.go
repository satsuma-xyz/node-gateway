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

	testForSingleRequest := func(methodName string, isStateRequired, isTraceMethod, isLogMethod bool) testArgs {
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
				IsLogMethod:     isLogMethod,
			},
		}
	}

	testForBatchRequest := func(testName string, methodNames []string, isStateRequired bool, isTraceMethod, isLogMethod bool) testArgs {
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
				IsLogMethod:     isLogMethod,
			},
		}
	}

	tests := []testArgs{
		testForSingleRequest("eth_call", true, false, false),
		testForSingleRequest("eth_getBalance", true, false, false),
		testForSingleRequest("eth_getBlockByNumber", false, false, false),
		testForSingleRequest("eth_getTransactionReceipt", false, false, false),
		testForSingleRequest("trace_filter", false, true, false),
		testForSingleRequest("trace_replayBlockTransactions", false, true, false),
		testForSingleRequest("eth_getLogs", false, false, true),
		testForBatchRequest("batch eth_call", []string{"eth_call"}, true, false, false),
		testForBatchRequest("batch eth_call w/ eth_getTransactionReceipt",
			[]string{"eth_call", "eth_getTransactionReceipt"}, true, false, false),
		testForBatchRequest("batch trace w/ eth_getTransactionReceipt",
			[]string{"trace_filter", "eth_getTransactionReceipt"}, false, true, false),
		testForBatchRequest("batch eth_call w/ eth_getTransactionReceipt, trace, and eth_getLogs",
			[]string{"eth_call", "eth_getTransactionReceipt", "trace_filter", "eth_getLogs"}, true, true, true),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RequestMetadataParser{}
			assert.Equalf(t, tt.want, p.Parse(tt.args.requestBody), "Parse(%v)", tt.args.requestBody)
		})
	}
}
