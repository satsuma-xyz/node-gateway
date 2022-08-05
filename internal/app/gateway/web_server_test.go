package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleJSONRPCRequestSuccess(t *testing.T) {
	emptyJSONBody, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handleJSONRPCRequest(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()
	statusCode := result.StatusCode

	if statusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, statusCode)
	}
}

func TestHandleJSONRPCRequestNonPost(t *testing.T) {
	emptyJSONBody, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handleJSONRPCRequest(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()
	statusCode := result.StatusCode

	if statusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, statusCode)
	}
}

func TestHandleJSONRPCRequestNonJSONContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("body")))
	req.Header.Add("Content-Type", "text/plain")

	recorder := httptest.NewRecorder()

	handleJSONRPCRequest(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()
	statusCode := result.StatusCode

	if statusCode != http.StatusUnsupportedMediaType {
		t.Errorf("Expected status code %d, got %d", http.StatusUnsupportedMediaType, statusCode)
	}
}

func TestHandleJSONRPCRequestBadJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("{\"bad_json\": ")))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handleJSONRPCRequest(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()
	statusCode := result.StatusCode

	if statusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, statusCode)
	}
}

func TestHandleJSONRPCRequestUnknownBodyField(t *testing.T) {
	emptyJSONBody, _ := json.Marshal(map[string]any{"unknown_field": "value"})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handleJSONRPCRequest(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()
	statusCode := result.StatusCode

	if statusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, statusCode)
	}
}
