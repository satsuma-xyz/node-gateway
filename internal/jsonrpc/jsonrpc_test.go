package jsonrpc

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeAndDecodeRequests(t *testing.T) {
	for _, tc := range []struct {
		expectedRequest RequestBody
		testName        string
		body            string
	}{
		{
			testName: "single request",
			body:     "{\"jsonrpc\":\"2.0\",\"method\":\"web3_clientVersion\",\"params\":[\"hi\"],\"id\":67}",
			expectedRequest: &SingleRequestBody{
				JSONRPCVersion: "2.0",
				Method:         "web3_clientVersion",
				Params:         []any{"hi"},
				ID:             67,
			},
		},
		{
			testName: "single request in batch",
			body:     "[{\"jsonrpc\":\"2.0\",\"method\":\"web3_clientVersion\",\"params\":[\"hi\"],\"id\":67}]",
			expectedRequest: &BatchRequestBody{
				Requests: []SingleRequestBody{
					{
						JSONRPCVersion: "2.0",
						Method:         "web3_clientVersion",
						Params:         []any{"hi"},
						ID:             67,
					},
				},
			},
		},
		{
			testName: "batch requests",
			body: "[" +
				"{\"jsonrpc\":\"2.0\",\"method\":\"web3_clientVersion\",\"params\":[\"hi\"],\"id\":67}," +
				"{\"jsonrpc\":\"2.0\",\"method\":\"web3_weee\",\"params\":[\"hi\"],\"id\":68}," +
				"{\"jsonrpc\":\"2.0\",\"method\":\"web3_something_else\",\"params\":[\"hello\"],\"id\":69}" +
				"]",
			expectedRequest: &BatchRequestBody{
				Requests: []SingleRequestBody{
					{
						JSONRPCVersion: "2.0",
						Method:         "web3_clientVersion",
						Params:         []any{"hi"},
						ID:             67,
					},
					{
						JSONRPCVersion: "2.0",
						Method:         "web3_weee",
						Params:         []any{"hi"},
						ID:             68,
					},
					{
						JSONRPCVersion: "2.0",
						Method:         "web3_something_else",
						Params:         []any{"hello"},
						ID:             69,
					},
				},
			},
		},
	} {
		req := http.Request{
			Body: io.NopCloser(bytes.NewReader([]byte(tc.body))),
		}
		decoded, err := DecodeRequestBody(&req)

		assert.Nil(t, err)
		assert.Equal(t, tc.expectedRequest, decoded)

		encoded, err := decoded.Encode()

		assert.Nil(t, err)
		assert.Equal(t, tc.body, string(encoded))
	}
}

func TestEncodeAndDecodeResponses(t *testing.T) {
	for _, tc := range []struct {
		expectedResponse ResponseBody
		testName         string
		body             string
	}{
		{
			testName: "single response",
			body:     "{\"result\":\"haha\",\"jsonrpc\":\"2.0\",\"id\":67}",
			expectedResponse: &SingleResponseBody{
				Result:  "haha",
				JSONRPC: "2.0",
				ID:      67,
			},
		},
		{
			testName: "single response in batch",
			body:     "[{\"result\":\"haha\",\"jsonrpc\":\"2.0\",\"id\":67}]",
			expectedResponse: &BatchResponseBody{
				Responses: []SingleResponseBody{
					{
						Result:  "haha",
						JSONRPC: "2.0",
						ID:      67,
					},
				},
			},
		},
		{
			testName: "batch responses",
			body: "[" +
				"{\"result\":\"haha\",\"jsonrpc\":\"2.0\",\"id\":67}," +
				"{\"result\":\"something\",\"jsonrpc\":\"2.0\",\"id\":68}," +
				"{\"result\":\"else\",\"jsonrpc\":\"2.0\",\"id\":69}" +
				"]",
			expectedResponse: &BatchResponseBody{
				Responses: []SingleResponseBody{
					{
						Result:  "haha",
						JSONRPC: "2.0",
						ID:      67,
					},
					{
						Result:  "something",
						JSONRPC: "2.0",
						ID:      68,
					},
					{
						Result:  "else",
						JSONRPC: "2.0",
						ID:      69,
					},
				},
			},
		},
	} {
		resp := http.Response{
			Body: io.NopCloser(bytes.NewReader([]byte(tc.body))),
		}

		decoded, err := DecodeResponseBody(&resp)

		assert.Nil(t, err)
		assert.Equal(t, tc.expectedResponse, decoded)

		encoded, err := decoded.Encode()

		assert.Nil(t, err)
		assert.Equal(t, tc.body, string(encoded))
	}
}
