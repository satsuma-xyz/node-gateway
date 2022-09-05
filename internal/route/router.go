package route

import (
	"bytes"
	"context"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"io"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/client"
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
	// Map from Priority => UpstreamIDs
	priorityToUpstreams map[int][]string
	upstreamConfigs     []config.UpstreamConfig
	requestExecutor     RequestExecutor
}

func NewRouter(upstreamConfigs []config.UpstreamConfig, groupConfigs []config.GroupConfig) Router {
	blockHeightChan := make(chan uint64)
	chainMetadataStore := metadata.NewChainMetadataStore(blockHeightChan)
	healthCheckManager := checks.NewHealthCheckManager(client.NewEthClient, upstreamConfigs, blockHeightChan)

	r := &SimpleRouter{
		chainMetadataStore:  chainMetadataStore,
		healthCheckManager:  healthCheckManager,
		upstreamConfigs:     upstreamConfigs,
		priorityToUpstreams: groupUpstreamsByPriority(upstreamConfigs, groupConfigs),
		routingStrategy:     NewPriorityRoundRobinStrategy(),
		requestExecutor:     RequestExecutor{httpClient: &http.Client{}},
	}

	return r
}

func groupUpstreamsByPriority(upstreamConfigs []config.UpstreamConfig, groupConfigs []config.GroupConfig) map[int][]string {
	priorityMap := make(map[int][]string)

	for _, upstreamConfig := range upstreamConfigs {
		groupID := upstreamConfig.GroupID

		groupPriority := 0
		// If groups are not specified, all upstreams will be on priority 0.
		for _, groupConfig := range groupConfigs {
			if groupConfig.ID == groupID {
				groupPriority = groupConfig.Priority
				break
			}
		}

		priorityMap[groupPriority] = append(priorityMap[groupPriority], upstreamConfig.ID)
	}

	return priorityMap
}

func (r *SimpleRouter) Start() {
	r.chainMetadataStore.Start()
	r.healthCheckManager.StartHealthChecks()
}

func (r *SimpleRouter) Route(ctx context.Context, requestBody jsonrpc.RequestBody) (*jsonrpc.ResponseBody, *http.Response, error) {
	healthyUpstreams := r.getHealthyUpstreamsByPriority()

	hasHealthyUpstreams := false

	for _, upstreamsAtPriority := range healthyUpstreams {
		if len(upstreamsAtPriority) > 0 {
			hasHealthyUpstreams = true
			break
		}
	}

	if !hasHealthyUpstreams {
		httpResp := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(bytes.NewBufferString("No healthy upstream")),
		}

		return nil, httpResp, nil
	}

	id := r.routingStrategy.RouteNextRequest(healthyUpstreams)

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

// Health checks need to be calculated by priority due to Block Height needs to be calculated by priority.
func (r *SimpleRouter) getHealthyUpstreamsByPriority() map[int][]string {
	priorityToHealthyUpstreams := make(map[int][]string)

	for priority, upstreamIDs := range r.priorityToUpstreams {
		zap.L().Debug("Determining healthy upstreams at priority.", zap.Int("priority", priority), zap.Any("upstreams", upstreamIDs))

		priorityToHealthyUpstreams[priority] = r.healthCheckManager.GetHealthyUpstreams(upstreamIDs)
	}

	return priorityToHealthyUpstreams
}
