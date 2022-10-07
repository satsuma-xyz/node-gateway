package route

import (
	"bytes"
	"context"
	"errors"

	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/types"

	"io"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/util"
)

// This contains logic on where and how to route the request.
// For now this is pretty simple, but in the future we'll have things like
// caching, rate limiting, API-based routing and more.

//go:generate mockery --output ../mocks --name Router
type Router interface {
	Start()
	IsInitialized() bool
	Route(ctx context.Context, batchRequest jsonrpc.BatchRequestBody) (*jsonrpc.BatchResponseBody, *http.Response, error)
}

type SimpleRouter struct {
	chainMetadataStore *metadata.ChainMetadataStore
	healthCheckManager checks.HealthCheckManager
	routingStrategy    RoutingStrategy
	requestExecutor    RequestExecutor
	// Map from Priority => UpstreamIDs
	priorityToUpstreams types.PriorityToUpstreamsMap
	metadataParser      metadata.RequestMetadataParser
	upstreamConfigs     []config.UpstreamConfig
}

func NewRouter(
	upstreamConfigs []config.UpstreamConfig,
	groupConfigs []config.GroupConfig,
	chainMetadataStore *metadata.ChainMetadataStore,
	healthCheckManager checks.HealthCheckManager,
	routingStrategy RoutingStrategy,
) Router {
	r := &SimpleRouter{
		chainMetadataStore:  chainMetadataStore,
		healthCheckManager:  healthCheckManager,
		upstreamConfigs:     upstreamConfigs,
		priorityToUpstreams: groupUpstreamsByPriority(upstreamConfigs, groupConfigs),
		routingStrategy:     routingStrategy,
		requestExecutor:     RequestExecutor{httpClient: &http.Client{}},
		metadataParser:      metadata.RequestMetadataParser{},
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
	batchRequest jsonrpc.BatchRequestBody,
) (*jsonrpc.BatchResponseBody, *http.Response, error) {
	requestMetadata := r.metadataParser.Parse(batchRequest)
	id, err := r.routingStrategy.RouteNextRequest(r.priorityToUpstreams, requestMetadata)

	if err != nil {
		switch {
		case errors.Is(err, ErrNoHealthyUpstreams):
			httpResp := &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(bytes.NewBufferString(err.Error())),
			}

			return nil, httpResp, nil
		default:
			return nil, nil, err
		}
	}

	var configToRoute config.UpstreamConfig

	for _, upstreamConfig := range r.upstreamConfigs {
		if upstreamConfig.ID == id {
			configToRoute = upstreamConfig
		}
	}

	rpcMethod := batchRequest.Requests[0].Method
	if batchRequest.IsOriginallyBatch {
		rpcMethod = "batch"
	}

	metrics.UpstreamRPCRequestsTotal.WithLabelValues(
		util.GetClientFromContext(ctx),
		id,
		configToRoute.HTTPURL,
		rpcMethod,
	).Inc()

	zap.L().Debug("Routing request to upstream.", zap.String("upstreamID", id), zap.Any("request", batchRequest), zap.String("client", util.GetClientFromContext(ctx)))

	go func() {
		for _, request := range batchRequest.Requests {
			metrics.UpstreamJSONRPCRequestsTotal.WithLabelValues(
				util.GetClientFromContext(ctx),
				id,
				configToRoute.HTTPURL,
				request.Method,
			).Inc()
		}
	}()

	start := time.Now()
	body, response, err := r.requestExecutor.routeToConfig(ctx, batchRequest, &configToRoute)
	HTTPReponseCode := ""

	if response != nil {
		HTTPReponseCode = strconv.Itoa(response.StatusCode)
	}

	if err != nil {
		metrics.UpstreamRPCRequestErrorsTotal.WithLabelValues(
			util.GetClientFromContext(ctx),
			id,
			configToRoute.HTTPURL,
			rpcMethod,
			HTTPReponseCode,
			"",
		).Inc()
	}

	if body != nil {
		// To help correlate request IDs to responses.
		// It's the responsibility of the client to provide unique IDs.
		reqIDToRequestMap := make(map[int]jsonrpc.RequestBody)
		for _, req := range batchRequest.Requests {
			reqIDToRequestMap[int(req.ID)] = req
		}

		for _, resp := range body.Responses {
			if resp.Error != nil {
				zap.L().Warn("Encountered error in upstream JSONRPC response.",
					zap.Any("request", batchRequest), zap.Any("error", resp.Error),
					zap.String("client", util.GetClientFromContext(ctx)))

				metrics.UpstreamJSONRPCRequestErrorsTotal.WithLabelValues(
					util.GetClientFromContext(ctx),
					id,
					configToRoute.HTTPURL,
					reqIDToRequestMap[resp.ID].Method,
					HTTPReponseCode,
					strconv.Itoa(resp.Error.Code),
				).Inc()
			}
		}
	}

	metrics.UpstreamRPCDuration.WithLabelValues(
		util.GetClientFromContext(ctx),
		configToRoute.ID,
		configToRoute.HTTPURL,
		rpcMethod,
		HTTPReponseCode,
		"",
	).Observe(time.Since(start).Seconds())

	return body, response, err
}
