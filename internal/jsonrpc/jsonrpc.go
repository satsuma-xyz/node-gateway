package jsonrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

type DecodeError struct {
	Err     error
	Content []byte // Content that couldn't be decoded.
}

func NewDecodeError(err error, content []byte) DecodeError {
	return DecodeError{
		Err:     err,
		Content: content,
	}
}

func (e DecodeError) Error() string {
	return fmt.Sprintf("decode error: %s, content: %s", e.Err.Error(), string(e.Content))
}

func DecodeResponseBody(resp *http.Response) (*ResponseBody, error) {
	// As per the spec, it is the caller's responsibility to close the response body.
	defer resp.Body.Close()
	responseRawBytes, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, NewDecodeError(err, responseRawBytes)
	}

	// Empty JSON RPC responses are valid for "Notifications" (requests without "ID") https://www.jsonrpc.org/specification#notification
	if len(responseRawBytes) == 0 {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(responseRawBytes))
	decoder.DisallowUnknownFields()

	var body ResponseBody

	if err = decoder.Decode(&body); err != nil {
		err = NewDecodeError(err, responseRawBytes)
	}

	return &body, err
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
