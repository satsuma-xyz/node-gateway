package route

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

// This contains logic on where and how to route the request.
// For now this is pretty simple, but in the future we'll have things like
// caching, rate limiting, API-based routing and more.

//go:generate mockery --output ../mocks --name Router
type Router interface {
	Start()
	Route(requestBody jsonrpc.RequestBody) (*jsonrpc.ResponseBody, *http.Response, error)
}

type SimpleRouter struct {
	healthCheckManager checks.HealthCheckManager
	routingStrategy    RoutingStrategy
	httpClient         client.HTTPClient
	// Map from Priority => UpstreamIDs
	priorityToUpstreams map[int][]string
	upstreamConfigs     []config.UpstreamConfig
}

func NewRouter(upstreamConfigs []config.UpstreamConfig, groupConfigs []config.GroupConfig) Router {
	healthCheckManager := checks.NewHealthCheckManager(client.NewEthClient, upstreamConfigs)

	r := &SimpleRouter{
		healthCheckManager:  healthCheckManager,
		upstreamConfigs:     upstreamConfigs,
		priorityToUpstreams: groupUpstreamsByPriority(upstreamConfigs, groupConfigs),
		routingStrategy:     NewPriorityRoundRobinStrategy(),
		httpClient:          &http.Client{},
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
	r.healthCheckManager.StartHealthChecks()
}

func (r *SimpleRouter) Route(requestBody jsonrpc.RequestBody) (*jsonrpc.ResponseBody, *http.Response, error) {
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

	zap.L().Debug("Routing request to upstream.", zap.String("upstreamID", id), zap.Any("request", requestBody))

	bodyBytes, err := requestBody.EncodeRequestBody()
	if err != nil {
		zap.L().Error("Could not serialize request.", zap.Any("request", requestBody), zap.Error(err))
		return nil, nil, err
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	if err != nil {
		zap.L().Error("Could not create new http request.", zap.Any("request", requestBody), zap.Error(err))
		return nil, nil, err
	}

	httpReq.Header.Set("content-type", "application/json")

	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(configToRoute.BasicAuthConfig.Username + ":" + configToRoute.BasicAuthConfig.Password))
	httpReq.Header.Set("Authorization", "Basic "+encodedCredentials)

	resp, err := r.httpClient.Do(httpReq)

	if err != nil {
		zap.L().Error("Error encountered when executing request.", zap.Any("request", requestBody), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := jsonrpc.DecodeResponseBody(resp)
	if err != nil {
		zap.L().Warn("Could not deserialize response.", zap.Any("request", requestBody), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, err
	}

	zap.L().Debug("Successfully routed request to upstream.", zap.String("upstreamID", id), zap.Any("request", requestBody), zap.Any("response", respBody))

	return respBody, resp, nil
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
