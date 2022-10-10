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
	GetSubRequests() []*SingleRequestBody
}

// See: https://www.jsonrpc.org/specification#request_object
type SingleRequestBody struct {
	JSONRPCVersion string `json:"jsonrpc,omitempty"`
	Method         string `json:"method,omitempty"`
	Params         []any  `json:"params,omitempty"`
	ID             int64  `json:"id,omitempty"`
}

func (b *SingleRequestBody) Encode() ([]byte, error) {
	return json.Marshal(b)
}

func (b *SingleRequestBody) GetMethod() string {
	return b.Method
}

func (b *SingleRequestBody) GetSubRequests() []*SingleRequestBody {
	return []*SingleRequestBody{b}
}

// All requests, even non-batch, are represented by BatchRequestBody for convenience.
// A non-batch request is characterized by len([]SingleRequestBody) == 1 and IsBatch == false.
type BatchRequestBody struct {
	Requests          []SingleRequestBody
	IsOriginallyBatch bool // This is required to distinguish clients that batch a single request.
}

func (b *BatchRequestBody) Encode() ([]byte, error) {
	return json.Marshal(b.Requests)
}

func (b *BatchRequestBody) GetMethod() string {
	return "batch"
}

func (b *BatchRequestBody) GetSubRequests() []*SingleRequestBody {
	result := make([]*SingleRequestBody, len(b.Requests))

	for index := range b.Requests {
		result = append(result, &b.Requests[index])
	}

	return result
}

func (b *BatchRequestBody) EncodeRequestBody() ([]byte, error) {
	if len(b.Requests) == 1 && !b.IsOriginallyBatch {
		return json.Marshal(b.Requests[0])
	}

	return json.Marshal(b.Requests)
}

// See: http://www.jsonrpc.org/specification#response_object
type ResponseBody struct {
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
}

// All responses, even non-batch, are represented by BatchResponseBody for convenience.
// A non-batch response is characterized by len([]ResponseBody) == 1 and IsBatch == false.
type BatchResponseBody struct {
	Responses []ResponseBody
	IsBatch   bool // This is required to distinguish clients that batch a single request.
}

func (b *BatchResponseBody) EncodeResponseBody() ([]byte, error) {
	if len(b.Responses) == 1 && !b.IsBatch {
		return json.Marshal(b.Responses[0])
	}

	return json.Marshal(b.Responses)
}

// See: http://www.jsonrpc.org/specification#error_object
type Error struct {
	Data    any    `json:"data,omitempty"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

type Decodable interface {
	SingleRequestBody | []SingleRequestBody | ResponseBody | []ResponseBody
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
		//return &BatchRequestBody{
		//	Requests:          []SingleRequestBody{*body},
		//	IsOriginallyBatch: false,
		//}, nil
	}

	var batchBody *[]SingleRequestBody

	if batchBody, err = decode[[]SingleRequestBody](requestRawBytes); err == nil {
		return &BatchRequestBody{
			Requests:          *batchBody,
			IsOriginallyBatch: true,
		}, nil
	}

	return nil, NewDecodeError(err, requestRawBytes)
}

func DecodeResponseBody(resp *http.Response) (*BatchResponseBody, error) {
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

	var body *ResponseBody

	// Try non-batch first as these are probably more common.
	if body, err = decode[ResponseBody](responseRawBytes); err == nil {
		return &BatchResponseBody{
			Responses: []ResponseBody{*body},
			IsBatch:   false,
		}, nil
	}

	var batchBody *[]ResponseBody

	if batchBody, err = decode[[]ResponseBody](responseRawBytes); err == nil {
		return &BatchResponseBody{
			Responses: *batchBody,
			IsBatch:   true,
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

func CreateErrorJSONRPCResponseBody(message string, jsonRPCStatusCode int) BatchResponseBody {
	responseBody := ResponseBody{
		JSONRPC: JSONRPCVersion,
		Error: &Error{
			Code:    jsonRPCStatusCode,
			Message: message,
		},
	}

	return BatchResponseBody{
		Responses: []ResponseBody{responseBody},
	}
}

func CreateErrorJSONRPCResponseBodyWithRequests(message string, jsonRPCStatusCode int, reqs []*SingleRequestBody) BatchResponseBody {
	responseBodies := make([]ResponseBody, 0, len(reqs))

	for _, req := range reqs {
		responseBody := ResponseBody{
			JSONRPC: JSONRPCVersion,
			Error: &Error{
				Code:    jsonRPCStatusCode,
				Message: message,
			},
			ID: int(req.ID),
		}
		responseBodies = append(responseBodies, responseBody)
	}

	return BatchResponseBody{
		Responses: responseBodies,
	}
}
