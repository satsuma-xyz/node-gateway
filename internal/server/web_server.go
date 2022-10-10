package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/metadata"

	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"

	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/route"
	"github.com/satsuma-data/node-gateway/internal/util"
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

func NewRPCServer(config conf.Config, rootLogger *zap.Logger) RPCServer {
	currentChainConfig := &config.Chains[0]
	childLogger := rootLogger.With(zap.String("chainName", currentChainConfig.ChainName))

	dependencyContainer := wireSingleChainDependencies(currentChainConfig, childLogger)
	router := dependencyContainer.router
	handler := &RPCHandler{
		router: router,
		logger: childLogger,
	}
	healthCheckHandler := &HealthCheckHandler{
		router: router,
		logger: childLogger,
	}

	port := defaultServerPort
	if config.Global.Port > 0 {
		port = config.Global.Port
	}

	mux := http.NewServeMux()
	mux.Handle("/health", healthCheckHandler)

	mux.Handle("/", metrics.InstrumentHandler(handler, dependencyContainer.metricsContainer))

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

type DependencyContainer struct {
	router           route.Router
	metricsContainer *metrics.Container
}

func wireSingleChainDependencies(config *conf.SingleChainConfig, logger *zap.Logger) *DependencyContainer {
	metricContainer := metrics.NewContainer(config.ChainName)
	chainMetadataStore := metadata.NewChainMetadataStore()
	ticker := time.NewTicker(checks.PeriodicHealthCheckInterval)
	healthCheckManager := checks.NewHealthCheckManager(client.NewEthClient, config.Upstreams, chainMetadataStore, ticker, metricContainer, logger)

	enabledNodeFilters := []route.NodeFilterType{route.Healthy, route.MaxHeightForGroup, route.SimpleStateOrTracePresent, route.NearGlobalMaxHeight}
	nodeFilter := route.CreateNodeFilter(enabledNodeFilters, healthCheckManager, chainMetadataStore, logger, &config.Routing)
	routingStrategy := route.FilteringRoutingStrategy{
		NodeFilter:      nodeFilter,
		BackingStrategy: route.NewPriorityRoundRobinStrategy(logger),
		Logger:          logger,
	}

	router := route.NewRouter(config.Upstreams, config.Groups, chainMetadataStore, healthCheckManager, &routingStrategy, metricContainer, logger)

	return &DependencyContainer{
		router:           router,
		metricsContainer: metricContainer,
	}
}

func (s *RPCServer) Start() error {
	s.router.Start()
	return s.httpServer.ListenAndServe()
}

func (s *RPCServer) Shutdown() error {
	return s.httpServer.Shutdown(context.Background())
}

type HealthCheckHandler struct {
	router route.Router
	logger *zap.Logger
}

func (h *HealthCheckHandler) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	if h.router.IsInitialized() {
		respondRaw(h.logger, writer, []byte("OK"), http.StatusOK)
	} else {
		respondRaw(h.logger, writer, []byte("Starting up"), http.StatusServiceUnavailable)
	}
}

type RPCHandler struct {
	router route.Router
	logger *zap.Logger
}

func (h *RPCHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		respondJSON(h.logger, writer, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	ctx := util.NewContext(context.Background(), getClientID(req))

	headerContentType := req.Header.Get("Content-Type")
	// Content-Type SHOULD be 'application/json-rpc' but MAY be
	// 'application/json' or 'application/jsonrequest'.
	// See https://www.jsonrpc.org/historical/json-rpc-over-http.html.
	if !slices.Contains([]string{"application/json", "application/json-rpc", "application/jsonrequest"}, headerContentType) {
		respondJSON(h.logger, writer, "Content-Type not supported.", http.StatusUnsupportedMediaType)
		return
	}

	requestBody, err := jsonrpc.DecodeRequestBody(req)
	if err != nil {
		errMsg := fmt.Sprintf("Request body could not be parsed, err: %s", err.Error())
		resp := jsonrpc.CreateErrorJSONRPCResponseBody(errMsg, jsonrpc.InternalServerErrorCode)
		h.logger.Error(errMsg)
		respondJSONRPC(h.logger, writer, resp, http.StatusBadRequest)

		return
	}

	h.logger.Debug("Request received.", zap.String("method", req.Method), zap.String("path", req.URL.Path), zap.String("query", req.URL.RawQuery), zap.Any("body", requestBody))

	respBody, resp, err := h.router.Route(ctx, requestBody)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		switch e := err.(type) {
		case jsonrpc.DecodeError:
			respondRaw(nil, writer, e.Content, http.StatusOK)
			return
		default:
			resp := jsonrpc.CreateErrorJSONRPCResponseBodyWithRequest(fmt.Sprintf("Request could not be routed, err: %s", err.Error()), jsonrpc.InternalServerErrorCode, requestBody)
			respondJSONRPC(h.logger, writer, resp, http.StatusInternalServerError)

			return
		}
	}

	respondJSONRPC(h.logger, writer, respBody, resp.StatusCode)

	h.logger.Debug("Request successfully routed.", zap.Any("requestBody", requestBody))
}

func getClientID(req *http.Request) string {
	// Try to get the id of the `client` via query parameter. Admittedly this is a little hacky, but it won't break
	// functionality as query params aren't used in JSON RPC.
	// The reason we're using query param is because client code may be hard to modify, e.g. graph-nodes, while using
	// query param is part of the RPC URL which is usually supplied as a config to the client.
	// A better solution is to pass the client via an HTTP header.
	if clientID := req.URL.Query().Get("client"); clientID != "" {
		return clientID
	}

	return "unknown"
}

func respondJSONRPC(
	logger *zap.Logger,
	writer http.ResponseWriter,
	response jsonrpc.ResponseBody,
	httpStatusCode int,
) {
	if response == nil {
		writer.WriteHeader(httpStatusCode)
		return
	}

	respBytes, err := response.Encode()
	if err != nil {
		logger.Error("Failed to serialize response.", zap.Error(err), zap.String("response", string(respBytes)))
		return
	}

	writer.Header().Set("Content-Type", "application/json")

	// Note: Call `WriteHeader` last otherwise headers won't get written.
	// See Header() on the http.ResponseWriter interface for more information.
	writer.WriteHeader(httpStatusCode)

	if i, err := writer.Write(respBytes); err != nil {
		logger.Error("Failed to write JSON RPC response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(respBytes)))
		return
	}
}

func respondJSON(logger *zap.Logger, writer http.ResponseWriter, message string, httpStatusCode int) {
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
		logger.Error("Failed to write response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(jsonResp)))
		return
	}
}

func respondRaw(logger *zap.Logger, writer http.ResponseWriter, body []byte, httpStatusCode int) {
	writer.WriteHeader(httpStatusCode)

	if i, err := writer.Write(body); err != nil {
		logger.Error("Failed to write raw response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(body)))
		return
	}
}
