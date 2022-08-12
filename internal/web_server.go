package internal

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/satsuma-data/node-gateway/internal/rpc"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

func StartServer(config Config) error {
	healthCheckManager := NewHealthCheckManager(NewEthClient)
	healthCheckManager.StartHealthChecks(config.Upstreams)

	router := NewRouter(healthCheckManager, config.Upstreams)

	server := NewServer(config, router)
	http.HandleFunc("/", server.handleJSONRPCRequest)

	return http.ListenAndServe(":8080", nil)
}

type Server struct {
	router Router
}

func NewServer(config Config, router Router) *Server {
	return &Server{
		router: router,
	}
}

func (s *Server) handleJSONRPCRequest(responseWriter http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		respond(responseWriter, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	headerContentType := req.Header.Get("Content-Type")
	// Content-Type SHOULD be 'application/json-rpc' but MAY be
	// 'application/json' or 'application/jsonrequest'.
	// See https://www.jsonrpc.org/historical/json-rpc-over-http.html.
	if !slices.Contains([]string{"application/json", "application/json-rpc", "application/jsonrequest"}, headerContentType) {
		respond(responseWriter, "Content-Type not supported.", http.StatusUnsupportedMediaType)
		return
	}

	body, err := rpc.DecodeRequestBody(req)
	if err != nil {
		resp := rpc.CreateErrorJSONRPCResponseBody(fmt.Sprintf("Request body could not be parsed, err: %s", err.Error()), rpc.InternalServerErrorCode, int(body.ID))
		respondJSONRPC(responseWriter, resp, http.StatusBadRequest)

		return
	}

	zap.L().Info("Request received.", zap.String("method", req.Method), zap.String("path", req.URL.Path), zap.String("query", req.URL.RawQuery), zap.Any("body", body))

	respBody, resp, err := s.router.Route(body)
	defer resp.Body.Close()

	if err != nil {
		resp := rpc.CreateErrorJSONRPCResponseBody(fmt.Sprintf("Request could not be routed, err: %s", err.Error()), rpc.InternalServerErrorCode, int(body.ID))
		respondJSONRPC(responseWriter, resp, http.StatusInternalServerError)

		return
	}

	respondJSONRPC(responseWriter, respBody, resp.StatusCode)

	zap.L().Debug("Request successfully routed", zap.Any("requestBody", body))
}

func respondJSONRPC(responseWriter http.ResponseWriter, response rpc.JSONRPCResponseBody, httpStatusCode int) {
	respBytes, err := response.EncodeResponseBody()
	if err != nil {
		zap.L().Error("Failed to serialize response.", zap.Error(err), zap.String("response", string(respBytes)))
		return
	}

	responseWriter.WriteHeader(httpStatusCode)
	responseWriter.Header().Set("Content-Type", "application/json")

	if i, err := responseWriter.Write(respBytes); err != nil {
		zap.L().Error("Failed to write response body", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(respBytes)))
		return
	}
}

func respond(responseWriter http.ResponseWriter, message string, httpStatusCode int) {
	resp := make(map[string]string)
	if message != "" {
		resp["message"] = message
	}

	responseWriter.WriteHeader(httpStatusCode)
	responseWriter.Header().Set("Content-Type", "application/json")

	jsonResp, _ := json.Marshal(resp)
	if i, err := responseWriter.Write(jsonResp); err != nil {
		zap.L().Error("Failed to write response body", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(jsonResp)))
		return
	}
}
