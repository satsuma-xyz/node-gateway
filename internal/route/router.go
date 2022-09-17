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
	Route(ctx context.Context, requestBody jsonrpc.RequestBody) (*jsonrpc.ResponseBody, *http.Response, error)
}

type SimpleRouter struct {
	chainMetadataStore *metadata.ChainMetadataStore
	healthCheckManager checks.HealthCheckManager
	routingStrategy    RoutingStrategy
	requestExecutor    RequestExecutor
	// Map from Priority => UpstreamIDs
	priorityToUpstreams types.PriorityToUpstreamsMap
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

func (r *SimpleRouter) Route(
	ctx context.Context,
	requestBody jsonrpc.RequestBody,
) (*jsonrpc.ResponseBody, *http.Response, error) {
	id, err := r.routingStrategy.RouteNextRequest(r.priorityToUpstreams)
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

	zap.L().Debug("Routing request to upstream.", zap.String("upstreamID", id), zap.Any("request", requestBody), zap.String("client", util.GetClientFromContext(ctx)))
	metrics.UpstreamRPCRequestsTotal.WithLabelValues(
		util.GetClientFromContext(ctx),
		id,
		configToRoute.HTTPURL,
		requestBody.Method,
	).Inc()

	start := time.Now()
	body, response, err := r.requestExecutor.routeToConfig(ctx, requestBody, &configToRoute)
	HTTPReponseCode := ""

	if response != nil {
		HTTPReponseCode = strconv.Itoa(response.StatusCode)
	}

	isJSONRPCError := body != nil && body.Error != nil
	if err != nil || isJSONRPCError {
		JSONRPCResponseCode := ""

		if isJSONRPCError {
			zap.L().Warn("Encountered error in upstream JSONRPC response.", zap.Any("request", requestBody), zap.Any("error", body.Error), zap.String("client", util.GetClientFromContext(ctx)))

			JSONRPCResponseCode = strconv.Itoa(body.Error.Code)
		}

		metrics.UpstreamRPCRequestErrorsTotal.WithLabelValues(
			util.GetClientFromContext(ctx),
			id,
			configToRoute.HTTPURL,
			requestBody.Method,
			HTTPReponseCode,
			JSONRPCResponseCode,
		).Inc()
	}

	metrics.UpstreamRPCDuration.WithLabelValues(
		util.GetClientFromContext(ctx),
		configToRoute.ID,
		configToRoute.HTTPURL,
		requestBody.Method,
		HTTPReponseCode,
		"",
	).Observe(time.Since(start).Seconds())

	return body, response, err
}
