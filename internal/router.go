package internal

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/exp/maps"

	"github.com/satsuma-data/node-gateway/internal/rpc"
)

// This contains logic on where and how to route the request.
// For now this is pretty simple, but in the future we'll have things like
// caching, rate limiting, API-based routing and more.

// go:generate mockery --output ./mocks --name Router
type Router interface {
	Route(requestBody rpc.JSONRPCRequestBody) (rpc.JSONRPCResponseBody, *http.Response, error)
}

type SimpleRouter struct {
	healthyNodesMutex  *sync.RWMutex
	healthCheckManager *HealthCheckManager
	routingStrategy    RoutingStrategy
	upstreamConfigs    []UpstreamConfig
}

func NewRouter(healthCheckManager *HealthCheckManager, upstreamConfigs []UpstreamConfig) Router {
	r := &SimpleRouter{
		healthCheckManager: healthCheckManager,
		upstreamConfigs:    upstreamConfigs,
		healthyNodesMutex:  &sync.RWMutex{},
		// Only support RoundRobin for now.
		routingStrategy: NewRoundRobinStrategy(),
	}
	r.startPollingHealthchecks()

	return r
}

// TODO: Make this configurable
const HealthyNodeSyncInterval = 1 * time.Second

func (r *SimpleRouter) startPollingHealthchecks() {
	go func() {
		for {
			r.healthyNodesMutex.Lock()
			healthyNodes := r.healthCheckManager.GetCurrentHealthyNodes()
			r.routingStrategy.updateNodeIDs(maps.Keys(healthyNodes))
			r.healthyNodesMutex.Unlock()

			time.Sleep(HealthyNodeSyncInterval)
		}
	}()
}

// Returns the JSONRPCResponseBody, HTTP status code, and error if encountered
func (r *SimpleRouter) Route(requestBody rpc.JSONRPCRequestBody) (rpc.JSONRPCResponseBody, *http.Response, error) {
	nodeID := r.routingStrategy.routeNextRequest()

	var configToRoute UpstreamConfig

	for _, config := range r.upstreamConfigs {
		if config.ID == nodeID {
			configToRoute = config
			break
		}
	}

	zap.L().Debug("Routing request to config.", zap.Any("request", requestBody), zap.Any("config", configToRoute))

	bodyBytes, err := requestBody.EncodeRequestBody()
	if err != nil {
		zap.L().Error("Could not serialize request", zap.Any("request", requestBody), zap.Error(err))
		return rpc.JSONRPCResponseBody{}, nil, err
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	if err != nil {
		zap.L().Error("Could not create new http request", zap.Any("request", requestBody), zap.Error(err))
		return rpc.JSONRPCResponseBody{}, nil, err
	}

	httpReq.Header.Set("content-type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)

	if err != nil {
		zap.L().Error("Error encountered when executing request", zap.Any("request", requestBody), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return rpc.JSONRPCResponseBody{}, nil, err
	}
	defer resp.Body.Close()

	respBody, err := rpc.DecodeResponseBody(resp)
	if err != nil {
		zap.L().Error("Could not deserialize response", zap.Any("request", requestBody), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return rpc.JSONRPCResponseBody{}, nil, err
	}

	zap.L().Debug("Successfully routed request to config.", zap.Any("request", requestBody), zap.Any("response", respBody), zap.Any("config", configToRoute))

	return respBody, resp, nil
}
