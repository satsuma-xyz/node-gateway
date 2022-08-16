package jsonrpc

import (
	"encoding/json"
	"net/http"
)

const JSONRPCVersion = "2.0"
const InternalServerErrorCode = -32000

// See: https://www.jsonrpc.org/specification#request_object
type RequestBody struct {
	JSONRPCVersion string `json:"jsonrpc,omitempty"`
	Method         string `json:"method,omitempty"`
	Params         []any  `json:"params,omitempty"`
	ID             int64  `json:"id,omitempty"`
}

func (b *RequestBody) EncodeRequestBody() ([]byte, error) {
	return json.Marshal(b)
}

func DecodeRequestBody(req *http.Request) (RequestBody, error) {
	// No need to close the request body, the Server implementation will take care of it.
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()

	var body RequestBody

	err := decoder.Decode(&body)

	return body, err
}

// See: http://www.jsonrpc.org/specification#response_object
type ResponseBody struct {
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
}

// See: http://www.jsonrpc.org/specification#error_object
type Error struct {
	Data    any    `json:"data,omitempty"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (b *ResponseBody) EncodeResponseBody() ([]byte, error) {
	return json.Marshal(b)
}

func DecodeResponseBody(resp *http.Response) (ResponseBody, error) {
	// As per the spec, it is the caller's responsibility to close the response body.
	defer resp.Body.Close()
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()

	var body ResponseBody

	err := decoder.Decode(&body)

	return body, err
}

func CreateErrorJSONRPCResponseBody(message string, jsonRPCStatusCode, id int) ResponseBody {
	return ResponseBody{
		JSONRPC: JSONRPCVersion,
		Error: &Error{
			Code:    jsonRPCStatusCode,
			Message: message,
		},
		ID: id,
	}
}
