package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/mocks"
)

func TestHandleJSONRPCRequest_Success(t *testing.T) {
	router := mocks.NewRouter(t)
	expectedRPCResponse := &jsonrpc.SingleResponseBody{
		JSONRPC: jsonrpc.JSONRPCVersion,
		Result:  "results",
		ID:      2,
	}
	router.On("Route", mock.Anything, mock.Anything).
		Return(expectedRPCResponse,
			&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("dummy")),
			}, nil)

	handler := &RPCHandler{router: router, logger: zap.L()}

	emptyJSONBody, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	jsonRPCResponse, _ := jsonrpc.DecodeResponseBody(result)

	assert.Equal(t, result.StatusCode, http.StatusOK)
	assert.Equal(t, expectedRPCResponse, jsonRPCResponse)
}

func TestHandleJSONRPCRequest_NonPost(t *testing.T) {
	router := mocks.NewRouter(t)
	handler := &RPCHandler{router: router}

	emptyJSONBody, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, result.StatusCode)
}

func TestHandleJSONRPCRequest_NonJSONContentType(t *testing.T) {
	router := mocks.NewRouter(t)
	handler := &RPCHandler{router: router}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("body")))
	req.Header.Add("Content-Type", "text/plain")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	assert.Equal(t, http.StatusUnsupportedMediaType, result.StatusCode)
}

func TestHandleJSONRPCRequest_BadJSON(t *testing.T) {
	router := mocks.NewRouter(t)
	handler := &RPCHandler{router: router, logger: zap.L()}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{\"bad_json\": ")))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	assert.Equal(t, http.StatusBadRequest, result.StatusCode)
}

func TestHandleJSONRPCRequest_UnknownBodyField(t *testing.T) {
	router := mocks.NewRouter(t)
	handler := &RPCHandler{router: router, logger: zap.L()}

	emptyJSONBody, _ := json.Marshal(map[string]any{"unknown_field": "value"})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	assert.Equal(t, http.StatusBadRequest, result.StatusCode)
}

func TestHandleJSONRPCRequest_NilJSONRPCResponse(t *testing.T) {
	router := mocks.NewRouter(t)

	router.On("Route", mock.Anything, mock.Anything).
		Return(nil,
			&http.Response{
				StatusCode: http.StatusAccepted,
				Body:       io.NopCloser(strings.NewReader("dummy")),
			}, nil)

	handler := &RPCHandler{router: router, logger: zap.L()}

	emptyJSONBody, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	assert.Equal(t, http.StatusAccepted, result.StatusCode)
	body, _ := io.ReadAll(result.Body)
	assert.Empty(t, body)
}

func TestHandleJSONRPCRequest_JSONRPCDecodeError(t *testing.T) {
	router := mocks.NewRouter(t)
	undecodableContent := []byte("content")

	router.On("Route", mock.Anything, mock.Anything).
		Return(nil, nil, jsonrpc.DecodeError{Err: errors.New("error decoding"), Content: undecodableContent})

	handler := &RPCHandler{router: router, logger: zap.L()}

	emptyJSONBody, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	assert.Equal(t, http.StatusOK, result.StatusCode)
	body, _ := io.ReadAll(result.Body)
	assert.Equal(t, undecodableContent, body)
}
