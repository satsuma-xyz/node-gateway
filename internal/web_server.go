package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

const (
	defaultServerPort        = 8080
	defaultReadHeaderTimeout = 10 * time.Second
)

type RPCServer struct {
	httpServer *http.Server
	router     Router
	config     conf.Config
}

func NewRPCServer(config conf.Config) RPCServer {
	router := NewRouter(config.Upstreams, config.Global.EnableHealthChecks)
	handler := &RPCHandler{
		router: router,
	}

	port := defaultServerPort
	if config.Global.Port > 0 {
		port = config.Global.Port
	}

	mux := http.NewServeMux()
	mux.Handle("/", handler)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	rpcServer := &RPCServer{
		httpServer: httpServer,
		router:     router,
		config:     config,
	}

	return *rpcServer
}

func (s *RPCServer) Start() error {
	s.router.Start()
	return s.httpServer.ListenAndServe()
}

func (s *RPCServer) Shutdown() error {
	return s.httpServer.Shutdown(context.Background())
}

type RPCHandler struct {
	router Router
}

func (h *RPCHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	metrics.RPCRequestsCounter.Inc()

	if req.Method != http.MethodPost {
		respond(writer, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	headerContentType := req.Header.Get("Content-Type")
	// Content-Type SHOULD be 'application/json-rpc' but MAY be
	// 'application/json' or 'application/jsonrequest'.
	// See https://www.jsonrpc.org/historical/json-rpc-over-http.html.
	if !slices.Contains([]string{"application/json", "application/json-rpc", "application/jsonrequest"}, headerContentType) {
		respond(writer, "Content-Type not supported.", http.StatusUnsupportedMediaType)
		return
	}

	body, err := jsonrpc.DecodeRequestBody(req)
	if err != nil {
		resp := jsonrpc.CreateErrorJSONRPCResponseBody(fmt.Sprintf("Request body could not be parsed, err: %s", err.Error()), jsonrpc.InternalServerErrorCode, int(body.ID))
		respondJSONRPC(writer, resp, http.StatusBadRequest)

		return
	}

	zap.L().Info("Request received.", zap.String("method", req.Method), zap.String("path", req.URL.Path), zap.String("query", req.URL.RawQuery), zap.Any("body", body))

	respBody, resp, err := h.router.Route(body)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		resp := jsonrpc.CreateErrorJSONRPCResponseBody(fmt.Sprintf("Request could not be routed, err: %s", err.Error()), jsonrpc.InternalServerErrorCode, int(body.ID))
		respondJSONRPC(writer, resp, http.StatusInternalServerError)

		return
	}

	respondJSONRPC(writer, respBody, resp.StatusCode)
}

func respondJSONRPC(writer http.ResponseWriter, response jsonrpc.ResponseBody, httpStatusCode int) {
	respBytes, err := response.EncodeResponseBody()
	if err != nil {
		zap.L().Error("Failed to serialize response.", zap.Error(err), zap.String("response", string(respBytes)))
		return
	}

	writer.WriteHeader(httpStatusCode)
	writer.Header().Set("Content-Type", "application/json")

	if i, err := writer.Write(respBytes); err != nil {
		zap.L().Error("Failed to write JSON RPC response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(respBytes)))
		return
	}
}

func respond(writer http.ResponseWriter, message string, httpStatusCode int) {
	resp := make(map[string]string)
	if message != "" {
		resp["message"] = message
	}

	writer.WriteHeader(httpStatusCode)
	writer.Header().Set("Content-Type", "application/json")

	jsonResp, _ := json.Marshal(resp)
	if i, err := writer.Write(jsonResp); err != nil {
		zap.L().Error("Failed to write response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(jsonResp)))
		return
	}
}
