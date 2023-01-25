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

	testForSingleRequest := func(methodName string) testArgs {
		return testArgs{
			args{
				requestBody: &jsonrpc.SingleRequestBody{
					Method: methodName,
				},
			},
			methodName,
			RequestMetadata{
				Methods: []string{methodName},
			},
		}
	}

	testForBatchRequest := func(testName string, methodNames []string) testArgs {
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
				Methods: methodNames,
			},
		}
	}

	tests := []testArgs{
		testForSingleRequest("eth_call"),
		testForBatchRequest("batch eth_call w/ eth_getTransactionReceipt, trace, and eth_getLogs",
			[]string{"eth_call", "eth_getTransactionReceipt", "trace_filter", "eth_getLogs"}),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &RequestMetadataParser{}
			assert.Equalf(t, tt.want, p.Parse(tt.args.requestBody), "Parse(%v)", tt.args.requestBody)
		})
	}
}
