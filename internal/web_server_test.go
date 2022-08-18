package internal

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/mocks"
)

func TestHandleJSONRPCRequest_Success(t *testing.T) {
	router := mocks.NewRouter(t)
	expectedRPCResponse := jsonrpc.ResponseBody{
		JSONRPC: jsonrpc.JSONRPCVersion,
		Result:  "results",
		ID:      2,
	}
	router.On("Route", mock.Anything).
		Return(expectedRPCResponse,
			&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("dummy")),
			}, nil)

	handler := &RPCHandler{router: router}

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
	handler := &RPCHandler{router: router}

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
	handler := &RPCHandler{router: router}

	emptyJSONBody, _ := json.Marshal(map[string]any{"unknown_field": "value"})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	assert.Equal(t, http.StatusBadRequest, result.StatusCode)
}
