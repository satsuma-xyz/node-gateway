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

type RequestBody interface {
	Encode() ([]byte, error)
	GetMethod() string
	GetSubRequests() []SingleRequestBody
}

// See: https://www.jsonrpc.org/specification#request_object
type SingleRequestBody struct {
	ID             *int64 `json:"id,omitempty"`
	JSONRPCVersion string `json:"jsonrpc,omitempty"`
	Method         string `json:"method,omitempty"`
	Params         []any  `json:"params,omitempty"`
}

func (b *SingleRequestBody) Encode() ([]byte, error) {
	return json.Marshal(b)
}

func (b *SingleRequestBody) GetMethod() string {
	return b.Method
}

func (b *SingleRequestBody) GetSubRequests() []SingleRequestBody {
	return []SingleRequestBody{*b}
}

type BatchRequestBody struct {
	Requests []SingleRequestBody
}

func (b *BatchRequestBody) Encode() ([]byte, error) {
	return json.Marshal(b.Requests)
}

func (b *BatchRequestBody) GetMethod() string {
	return "batch"
}

func (b *BatchRequestBody) GetSubRequests() []SingleRequestBody {
	return append([]SingleRequestBody(nil), b.Requests...)
}

type ResponseBody interface {
	Encode() ([]byte, error)
	GetSubResponses() []SingleResponseBody
}

// See: http://www.jsonrpc.org/specification#response_object
type SingleResponseBody struct {
	Error   *Error          `json:"error,omitempty"`
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	ID      int64           `json:"id"`
}

func (b *SingleResponseBody) Encode() ([]byte, error) {
	return json.Marshal(b)
}

func (b *SingleResponseBody) GetSubResponses() []SingleResponseBody {
	return []SingleResponseBody{*b}
}

type BatchResponseBody struct {
	Responses []SingleResponseBody
}

func (b *BatchResponseBody) Encode() ([]byte, error) {
	return json.Marshal(b.Responses)
}

func (b *BatchResponseBody) GetSubResponses() []SingleResponseBody {
	return append([]SingleResponseBody(nil), b.Responses...)
}

// See: http://www.jsonrpc.org/specification#error_object
type Error struct {
	Data    any    `json:"data,omitempty"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type Decodable interface {
	SingleRequestBody | []SingleRequestBody | SingleResponseBody | []SingleResponseBody
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

func DecodeRequestBody(req *http.Request) (RequestBody, error) {
	// No need to close the request body, the Server implementation will take care of it.
	requestRawBytes, err := io.ReadAll(req.Body)

	if err != nil {
		return nil, NewDecodeError(err, requestRawBytes)
	}

	var body *SingleRequestBody

	// Try non-batch first as these are probably more common.
	if body, err = decode[SingleRequestBody](requestRawBytes); err == nil {
		return body, nil
	}

	var batchBody *[]SingleRequestBody

	if batchBody, err = decode[[]SingleRequestBody](requestRawBytes); err == nil {
		return &BatchRequestBody{
			Requests: *batchBody,
		}, nil
	}

	return nil, NewDecodeError(err, requestRawBytes)
}

func DecodeResponseBody(resp *http.Response) (ResponseBody, error) {
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

	var body *SingleResponseBody

	// Try non-batch first as these are probably more common.
	if body, err = decode[SingleResponseBody](responseRawBytes); err == nil {
		return body, nil
	}

	var batchBody *[]SingleResponseBody

	if batchBody, err = decode[[]SingleResponseBody](responseRawBytes); err == nil {
		return &BatchResponseBody{
			Responses: *batchBody,
		}, nil
	}

	return nil, NewDecodeError(err, responseRawBytes)
}

func decode[T Decodable](rawBytes []byte) (*T, error) {
	decoder := json.NewDecoder(bytes.NewReader(rawBytes))
	decoder.DisallowUnknownFields()

	var body T

	if err := decoder.Decode(&body); err != nil {
		return nil, NewDecodeError(err, rawBytes)
	}

	return &body, nil
}

func CreateErrorJSONRPCResponseBody(message string, jsonRPCStatusCode int) *SingleResponseBody {
	return &SingleResponseBody{
		JSONRPC: JSONRPCVersion,
		Error: &Error{
			Code:    jsonRPCStatusCode,
			Message: message,
		},
	}
}

func CreateErrorJSONRPCResponseBodyWithRequest(message string, jsonRPCStatusCode int, request RequestBody) ResponseBody {
	switch r := request.(type) {
	case *SingleRequestBody:
		response := CreateErrorJSONRPCResponseBody(message, jsonRPCStatusCode)
		if r.ID != nil {
			response.ID = *r.ID
		}

		return response
	case *BatchRequestBody:
		subRequests := r.GetSubRequests()
		responses := make([]SingleResponseBody, 0, len(subRequests))

		for _, subReq := range subRequests {
			response := SingleResponseBody{
				JSONRPC: subReq.JSONRPCVersion,
				Error: &Error{
					Code:    jsonRPCStatusCode,
					Message: message,
				},
				ID: *subReq.ID,
			}
			responses = append(responses, response)
		}

		return &BatchResponseBody{
			Responses: responses,
		}
	default:
		response := CreateErrorJSONRPCResponseBody(message, jsonRPCStatusCode)
		return response
	}
}
