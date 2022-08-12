package rpc

import (
	"encoding/json"
	"net/http"
)

const JSONRPCVersion = "2.0"
const InternalServerErrorCode = -32000

// See: https://www.jsonrpc.org/specification#request_object
type JSONRPCRequestBody struct {
	JSONRPCVersion string `json:"jsonrpc,omitempty"`
	Method         string `json:"method,omitempty"`
	Params         []any  `json:"params,omitempty"`
	ID             int64  `json:"id,omitempty"`
}

func (b *JSONRPCRequestBody) EncodeRequestBody() ([]byte, error) {
	return json.Marshal(b)
}

func DecodeRequestBody(req *http.Request) (JSONRPCRequestBody, error) {
	// No need to close the request body, the Server implementation will take care of it.
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()

	var body JSONRPCRequestBody

	err := decoder.Decode(&body)

	return body, err
}

// See: http://www.jsonrpc.org/specification#response_object
type JSONRPCResponseBody struct {
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
}

// See: http://www.jsonrpc.org/specification#error_object
type JSONRPCError struct {
	Data    any    `json:"data,omitempty"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (b *JSONRPCResponseBody) EncodeResponseBody() ([]byte, error) {
	return json.Marshal(b)
}

func DecodeResponseBody(resp *http.Response) (JSONRPCResponseBody, error) {
	// As per the spec, it is the caller's responsibility to close the response body.
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()

	var body JSONRPCResponseBody

	err := decoder.Decode(&body)

	return body, err
}

func CreateErrorJSONRPCResponseBody(message string, jsonRPCStatusCode, id int) JSONRPCResponseBody {
	return JSONRPCResponseBody{
		JSONRPC: JSONRPCVersion,
		Error: &JSONRPCError{
			Code:    jsonRPCStatusCode,
			Message: message,
		},
		ID: id,
	}
}
