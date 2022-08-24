package route

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sync"

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
	upstreamsMutex     *sync.RWMutex
	healthCheckManager checks.HealthCheckManager
	routingStrategy    RoutingStrategy
	// Map from UpstreamID => `config.UpstreamConfig`
	upstreamConfigs map[string]config.UpstreamConfig
	// Map from GroupID => `config.GroupConfig`
	groupConfigs map[string]config.GroupConfig
}

func NewRouter(upstreamConfigs []config.UpstreamConfig, groupConfigs []config.GroupConfig) Router {
	healthCheckManager := checks.NewHealthCheckManager(client.NewEthClient, upstreamConfigs)

	upstreamConfigMap := make(map[string]config.UpstreamConfig)
	for _, upstreamConfig := range upstreamConfigs {
		upstreamConfigMap[upstreamConfig.ID] = upstreamConfig
	}

	groupMap := make(map[string]config.GroupConfig)
	for _, groupConfig := range groupConfigs {
		groupMap[groupConfig.ID] = groupConfig
	}

	r := &SimpleRouter{
		healthCheckManager: healthCheckManager,
		upstreamConfigs:    upstreamConfigMap,
		groupConfigs:       groupMap,
		upstreamsMutex:     &sync.RWMutex{},
		routingStrategy:    NewPriorityRoundRobinStrategy(),
	}

	return r
}

func (r *SimpleRouter) Start() {
	r.healthCheckManager.StartHealthChecks()
}

func (r *SimpleRouter) Route(requestBody jsonrpc.RequestBody) (*jsonrpc.ResponseBody, *http.Response, error) {
	healthyUpstreams := r.healthCheckManager.GetHealthyUpstreams()

	if len(healthyUpstreams) == 0 {
		httpResp := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(bytes.NewBufferString("No healthy upstream")),
		}

		return nil, httpResp, nil
	}

	upstreamsByPriority := groupUpstreamsByPriority(healthyUpstreams, r.upstreamConfigs, r.groupConfigs)
	id := r.routingStrategy.routeNextRequest(upstreamsByPriority)
	configToRoute := r.upstreamConfigs[id]

	zap.L().Debug("Routing request to config.", zap.Any("request", requestBody), zap.Any("config", configToRoute))

	bodyBytes, err := requestBody.EncodeRequestBody()
	if err != nil {
		zap.L().Error("Could not serialize request", zap.Any("request", requestBody), zap.Error(err))
		return nil, nil, err
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	if err != nil {
		zap.L().Error("Could not create new http request", zap.Any("request", requestBody), zap.Error(err))
		return nil, nil, err
	}

	httpReq.Header.Set("content-type", "application/json")

	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(configToRoute.BasicAuthConfig.Username + ":" + configToRoute.BasicAuthConfig.Password))
	httpReq.Header.Set("Authorization", "Basic "+encodedCredentials)

	httpClient := &http.Client{}
	resp, err := httpClient.Do(httpReq)

	if err != nil {
		zap.L().Error("Error encountered when executing request", zap.Any("request", requestBody), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := jsonrpc.DecodeResponseBody(resp)
	if err != nil {
		zap.L().Error("Could not deserialize response", zap.Any("request", requestBody), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, err
	}

	zap.L().Debug("Successfully routed request to config.", zap.Any("request", requestBody), zap.Any("response", respBody), zap.Any("config", configToRoute))

	return respBody, resp, nil
}

func groupUpstreamsByPriority(upstreams []string, upstreamConfigs map[string]config.UpstreamConfig, groupConfigs map[string]config.GroupConfig) map[int][]string {
	priorityMap := make(map[int][]string)
	usingGroups := len(groupConfigs) > 0

	for _, upstream := range upstreams {
		groupID := upstreamConfigs[upstream].GroupID

		groupPriority := 0
		if usingGroups {
			groupPriority = groupConfigs[groupID].Priority
		}

		priorityMap[groupPriority] = append(priorityMap[groupPriority], upstream)
	}

	return priorityMap
}
