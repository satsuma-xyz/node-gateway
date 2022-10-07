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
		testName       string
		body           string
		objectsDecoded int
	}{
		{
			testName:       "single request",
			body:           "{\"jsonrpc\":\"2.0\",\"method\":\"web3_clientVersion\",\"params\":[1],\"id\":67}",
			objectsDecoded: 1,
		},
		{
			testName:       "single request in batch",
			body:           "[{\"jsonrpc\":\"2.0\",\"method\":\"web3_clientVersion\",\"params\":[1],\"id\":67}]",
			objectsDecoded: 1,
		},
		{
			testName: "batch requests",
			body: "[" +
				"{\"jsonrpc\":\"2.0\",\"method\":\"web3_clientVersion\",\"params\":[1],\"id\":67}," +
				"{\"jsonrpc\":\"2.0\",\"method\":\"web3_weee\",\"params\":[1],\"id\":68}," +
				"{\"jsonrpc\":\"2.0\",\"method\":\"web3_something_else\",\"params\":[2],\"id\":69}" +
				"]",
			objectsDecoded: 3,
		},
	} {
		req := http.Request{
			Body: io.NopCloser(bytes.NewReader([]byte(tc.body))),
		}
		decoded, err := DecodeRequestBody(&req)

		assert.Nil(t, err)
		assert.Equal(t, tc.objectsDecoded, len(decoded.Requests))

		encoded, err := decoded.EncodeRequestBody()

		assert.Nil(t, err)
		assert.Equal(t, tc.body, string(encoded))
	}
}

func TestEncodeAndDecodeResponses(t *testing.T) {
	for _, tc := range []struct {
		testName       string
		body           string
		objectsDecoded int
	}{
		{
			testName:       "single response",
			body:           "{\"result\":\"haha\",\"jsonrpc\":\"2.0\",\"id\":67}",
			objectsDecoded: 1,
		},
		{
			testName:       "single response in batch",
			body:           "[{\"result\":\"haha\",\"jsonrpc\":\"2.0\",\"id\":67}]",
			objectsDecoded: 1,
		},
		{
			testName: "batch responses",
			body: "[" +
				"{\"result\":\"haha\",\"jsonrpc\":\"2.0\",\"id\":67}," +
				"{\"result\":\"something\",\"jsonrpc\":\"2.0\",\"id\":68}," +
				"{\"result\":\"else\",\"jsonrpc\":\"2.0\",\"id\":69}" +
				"]",
			objectsDecoded: 3,
		},
	} {
		resp := http.Response{
			Body: io.NopCloser(bytes.NewReader([]byte(tc.body))),
		}

		decoded, err := DecodeResponseBody(&resp)

		assert.Nil(t, err)
		assert.Equal(t, tc.objectsDecoded, len(decoded.Responses))

		encoded, err := decoded.EncodeResponseBody()

		assert.Nil(t, err)
		assert.Equal(t, tc.body, string(encoded))
	}
}
