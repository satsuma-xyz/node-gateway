package gateway

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

type JSONRPCRequestBody struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      int64  `json:"id"`
}

func StartServer() error {
	http.HandleFunc("/", handleJSONRPCRequest)
	return http.ListenAndServe(":8080", nil)
}

func handleJSONRPCRequest(responseWriter http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		_ = respond(responseWriter, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	headerContentType := req.Header.Get("Content-Type")
	if headerContentType != "application/json" {
		_ = respond(responseWriter, "Content-Type must be application/json.", http.StatusUnsupportedMediaType)
		return
	}

	var body JSONRPCRequestBody

	decoder := json.NewDecoder(req.Body)

	decoder.DisallowUnknownFields()
	err := decoder.Decode(&body)

	if err != nil {
		_ = respond(responseWriter, "Request body could not be parsed. "+err.Error(), http.StatusBadRequest)
		return
	}

	zap.L().Info("Request received.", zap.String("method", req.Method), zap.String("path", req.URL.Path), zap.String("query", req.URL.RawQuery), zap.Any("body", body))
}

func respond(responseWriter http.ResponseWriter, message string, httpStatusCode int) error {
	responseWriter.WriteHeader(httpStatusCode)

	if message == "" {
		return nil
	}

	responseWriter.Header().Set("Content-Type", "application/json")

	resp := make(map[string]string)

	resp["message"] = message
	jsonResp, _ := json.Marshal(resp)
	_, err := responseWriter.Write(jsonResp)

	if err != nil {
		zap.L().Error("Failed to write response.", zap.Error(err))
	}

	return err
}
