package route

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/cache"
	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
	"github.com/satsuma-data/node-gateway/internal/util"
)

// This contains logic on where and how to route the request.
// For now this is pretty simple, but in the future we'll have things like
// caching, rate limiting, API-based routing and more.

//go:generate mockery --output ../mocks --name Router --with-expecter
type Router interface {
	Start()
	IsInitialized() bool
	Route(ctx context.Context, requestBody jsonrpc.RequestBody) (string, jsonrpc.ResponseBody, error)
}

type SimpleRouter struct {
	metadataParser      metadata.RequestMetadataParser
	requestExecutor     RequestExecutor
	healthCheckManager  checks.HealthCheckManager
	routingStrategy     RoutingStrategy
	chainMetadataStore  *metadata.ChainMetadataStore
	metricsContainer    *metrics.Container
	logger              *zap.Logger
	priorityToUpstreams types.PriorityToUpstreamsMap
	upstreamConfigs     []config.UpstreamConfig
}

func NewRouter(
	chainName string,
	cacheConfig config.ChainCacheConfig,
	upstreamConfigs []config.UpstreamConfig,
	groupConfigs []config.GroupConfig,
	chainMetadataStore *metadata.ChainMetadataStore,
	healthCheckManager checks.HealthCheckManager,
	routingStrategy RoutingStrategy,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
	rpcCache *cache.RPCCache,
) Router {
	r := &SimpleRouter{
		chainMetadataStore:  chainMetadataStore,
		healthCheckManager:  healthCheckManager,
		upstreamConfigs:     upstreamConfigs,
		priorityToUpstreams: groupUpstreamsByPriority(upstreamConfigs, groupConfigs),
		routingStrategy:     routingStrategy,
		requestExecutor:     RequestExecutor{&http.Client{}, cacheConfig, logger, rpcCache, chainName},
		metadataParser:      metadata.RequestMetadataParser{},
		metricsContainer:    metricsContainer,
		logger:              logger,
	}

	return r
}

func groupUpstreamsByPriority(
	upstreamConfigs []config.UpstreamConfig,
	groupConfigs []config.GroupConfig,
) types.PriorityToUpstreamsMap {
	priorityMap := make(types.PriorityToUpstreamsMap)

	for configIndex := range upstreamConfigs {
		upstreamConfig := &upstreamConfigs[configIndex]
		groupID := upstreamConfig.GroupID

		groupPriority := 0
		// If groups are not specified, all upstreams will be on priority 0.
		for _, groupConfig := range groupConfigs {
			if groupConfig.ID == groupID {
				groupPriority = groupConfig.Priority
				break
			}
		}

		priorityMap[groupPriority] = append(priorityMap[groupPriority], upstreamConfig)
	}

	return priorityMap
}

func (r *SimpleRouter) Start() {
	r.chainMetadataStore.Start()
	r.healthCheckManager.StartHealthChecks()
}

func (r *SimpleRouter) IsInitialized() bool {
	return r.healthCheckManager.IsInitialized()
}

func (r *SimpleRouter) Route(
	ctx context.Context,
	requestBody jsonrpc.RequestBody,
) (string, jsonrpc.ResponseBody, error) {
	requestMetadata := r.metadataParser.Parse(requestBody)
	upstreamID, err := r.routingStrategy.RouteNextRequest(r.priorityToUpstreams, requestMetadata)

	if err != nil {
		return "", nil, err
	}

	var configToRoute config.UpstreamConfig

	for idx := range r.upstreamConfigs {
		if r.upstreamConfigs[idx].ID == upstreamID {
			configToRoute = r.upstreamConfigs[idx]
		}
	}

	r.logger.Debug("Routing request to upstream.", zap.String("upstreamID", upstreamID), zap.Any("request", requestBody), zap.String("client", util.GetClientFromContext(ctx)))

	start := time.Now()
	jsonRPCResponse, httpResponse, cached, err := r.requestExecutor.routeToConfig(ctx, requestBody, &configToRoute)
	statusCode := 0
	HTTPResponseCode := ""

	if httpResponse != nil {
		statusCode = httpResponse.StatusCode
		HTTPResponseCode = strconv.Itoa(statusCode)
	}

	r.healthCheckManager.RecordRequest(upstreamID, &types.RequestData{
		Method:           requestBody.GetMethod(),
		HTTPResponseCode: statusCode,
		ResponseBody:     jsonRPCResponse,
		Latency:          time.Since(start),
		Error:            err,
	})

	r.metricsContainer.UpstreamRPCRequestsTotal.WithLabelValues(
		util.GetClientFromContext(ctx),
		upstreamID,
		configToRoute.HTTPURL,
		requestBody.GetMethod(),
		strconv.FormatBool(cached),
	).Inc()

	if err != nil || statusCode == 0 || statusCode >= http.StatusBadRequest {
		r.metricsContainer.UpstreamRPCRequestErrorsTotal.WithLabelValues(
			util.GetClientFromContext(ctx),
			upstreamID,
			configToRoute.HTTPURL,
			requestBody.GetMethod(),
			HTTPResponseCode,
		).Inc()
	}

	if jsonRPCResponse != nil {
		// To help correlate request IDs to responses.
		// It's the responsibility of the client to provide unique IDs.
		reqIDToRequestMap := make(map[int64]jsonrpc.SingleRequestBody)

		for _, req := range requestBody.GetSubRequests() {
			// JSONRPC requests without ID should not have a response.
			// Defensively checking here to avoid dereferencing nil pointer.
			if req.ID == nil {
				continue
			}

			reqIDToRequestMap[*req.ID] = req
		}

		for _, resp := range jsonRPCResponse.GetSubResponses() {
			if resp.Error != nil {
				r.logger.Warn("Encountered error in upstream JSONRPC response.",
					zap.Any("request", requestBody), zap.Any("error", resp.Error),
					zap.String("client", util.GetClientFromContext(ctx)), zap.String("upstreamID", upstreamID))

				// In the rare case that the response has an ID that does not have a corresponding request.
				if _, ok := reqIDToRequestMap[resp.ID]; !ok {
					continue
				}

				r.metricsContainer.UpstreamJSONRPCRequestErrorsTotal.WithLabelValues(
					util.GetClientFromContext(ctx),
					upstreamID,
					configToRoute.HTTPURL,
					reqIDToRequestMap[resp.ID].Method,
					strconv.Itoa(resp.Error.Code),
				).Inc()
			}
		}
	}

	r.metricsContainer.UpstreamRPCDuration.WithLabelValues(
		util.GetClientFromContext(ctx),
		configToRoute.ID,
		configToRoute.HTTPURL,
		requestBody.GetMethod(),
		HTTPResponseCode,
	).Observe(time.Since(start).Seconds())

	return upstreamID, jsonRPCResponse, err
}
