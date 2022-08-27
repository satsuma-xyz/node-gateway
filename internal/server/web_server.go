package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/route"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

const (
	defaultServerPort        = 8080
	defaultReadHeaderTimeout = 10 * time.Second
)

type RPCServer struct {
	httpServer *http.Server
	router     route.Router
	config     conf.Config
}

func NewRPCServer(config conf.Config) RPCServer {
	router := route.NewRouter(config.Upstreams, config.Groups)
	handler := &RPCHandler{
		router: router,
	}

	port := defaultServerPort
	if config.Global.Port > 0 {
		port = config.Global.Port
	}

	mux := http.NewServeMux()
	mux.Handle("/", metrics.InstrumentHandler(handler))

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
	router route.Router
}

func (h *RPCHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		respondJSON(writer, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	headerContentType := req.Header.Get("Content-Type")
	// Content-Type SHOULD be 'application/json-rpc' but MAY be
	// 'application/json' or 'application/jsonrequest'.
	// See https://www.jsonrpc.org/historical/json-rpc-over-http.html.
	if !slices.Contains([]string{"application/json", "application/json-rpc", "application/jsonrequest"}, headerContentType) {
		respondJSON(writer, "Content-Type not supported.", http.StatusUnsupportedMediaType)
		return
	}

	body, err := jsonrpc.DecodeRequestBody(req)
	if err != nil {
		resp := jsonrpc.CreateErrorJSONRPCResponseBody(fmt.Sprintf("Request body could not be parsed, err: %s", err.Error()), jsonrpc.InternalServerErrorCode, int(body.ID))
		respondJSONRPC(writer, &resp, http.StatusBadRequest)

		return
	}

	zap.L().Debug("Request received.", zap.String("method", req.Method), zap.String("path", req.URL.Path), zap.String("query", req.URL.RawQuery), zap.Any("body", body))

	respBody, resp, err := h.router.Route(body)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		switch e := err.(type) {
		case jsonrpc.DecodeError:
			respondRaw(writer, e.Content, http.StatusOK)
			return
		default:
			resp := jsonrpc.CreateErrorJSONRPCResponseBody(fmt.Sprintf("Request could not be routed, err: %s", err.Error()), jsonrpc.InternalServerErrorCode, int(body.ID))
			respondJSONRPC(writer, &resp, http.StatusInternalServerError)

			return
		}
	}

	respondJSONRPC(writer, respBody, resp.StatusCode)

	zap.L().Debug("Request successfully routed.", zap.Any("requestBody", body))
}

func respondJSONRPC(writer http.ResponseWriter, response *jsonrpc.ResponseBody, httpStatusCode int) {
	if response == nil {
		writer.WriteHeader(httpStatusCode)
		return
	}

	respBytes, err := response.EncodeResponseBody()
	if err != nil {
		zap.L().Error("Failed to serialize response.", zap.Error(err), zap.String("response", string(respBytes)))
		return
	}

	writer.Header().Set("Content-Type", "application/json")

	// Note: Call `WriteHeader` last otherwise headers won't get written.
	// See Header() on the http.ResponseWriter interface for more information.
	writer.WriteHeader(httpStatusCode)

	if i, err := writer.Write(respBytes); err != nil {
		zap.L().Error("Failed to write JSON RPC response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(respBytes)))
		return
	}
}

func respondJSON(writer http.ResponseWriter, message string, httpStatusCode int) {
	resp := make(map[string]string)
	if message != "" {
		resp["message"] = message
	}

	writer.Header().Set("Content-Type", "application/json")

	// Note: Call `WriteHeader` last otherwise headers won't get written.
	// See Header() on the http.ResponseWriter interface for more information.
	writer.WriteHeader(httpStatusCode)

	jsonResp, _ := json.Marshal(resp)
	if i, err := writer.Write(jsonResp); err != nil {
		zap.L().Error("Failed to write response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(jsonResp)))
		return
	}
}

func respondRaw(writer http.ResponseWriter, body []byte, httpStatusCode int) {
	writer.WriteHeader(httpStatusCode)

	if i, err := writer.Write(body); err != nil {
		zap.L().Error("Failed to write raw response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(body)))
		return
	}
}
