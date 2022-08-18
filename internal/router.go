package internal

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sync"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

// This contains logic on where and how to route the request.
// For now this is pretty simple, but in the future we'll have things like
// caching, rate limiting, API-based routing and more.

//go:generate mockery --output ./mocks --name Router
type Router interface {
	Start()
	Route(requestBody jsonrpc.RequestBody) (jsonrpc.ResponseBody, *http.Response, error)
}

type SimpleRouter struct {
	upstreamsMutex     *sync.RWMutex
	healthCheckManager HealthCheckManager
	routingStrategy    RoutingStrategy
	upstreamConfigs    []UpstreamConfig
}

func NewRouter(upstreamConfigs []UpstreamConfig) Router {
	healthCheckManager := NewHealthCheckManager(NewEthClient, upstreamConfigs)

	r := &SimpleRouter{
		healthCheckManager: healthCheckManager,
		upstreamConfigs:    upstreamConfigs,
		upstreamsMutex:     &sync.RWMutex{},
		// Only support RoundRobin for now.
		routingStrategy: NewRoundRobinStrategy(),
	}

	return r
}

func (r *SimpleRouter) Start() {
	r.healthCheckManager.StartHealthChecks()
}

func (r *SimpleRouter) Route(requestBody jsonrpc.RequestBody) (jsonrpc.ResponseBody, *http.Response, error) {
	healthyUpstreams := r.healthCheckManager.GetHealthyUpstreams()
	if len(healthyUpstreams) == 0 {
		httpResp := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(bytes.NewBufferString("No healthy upstream")),
		}

		return jsonrpc.ResponseBody{}, httpResp, nil
	}

	id := r.routingStrategy.routeNextRequest(healthyUpstreams)

	var configToRoute UpstreamConfig

	for _, config := range r.upstreamConfigs {
		if config.ID == id {
			configToRoute = config
			break
		}
	}

	zap.L().Debug("Routing request to config.", zap.Any("request", requestBody), zap.Any("config", configToRoute))

	bodyBytes, err := requestBody.EncodeRequestBody()
	if err != nil {
		zap.L().Error("Could not serialize request", zap.Any("request", requestBody), zap.Error(err))
		return jsonrpc.ResponseBody{}, nil, err
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	if err != nil {
		zap.L().Error("Could not create new http request", zap.Any("request", requestBody), zap.Error(err))
		return jsonrpc.ResponseBody{}, nil, err
	}

	httpReq.Header.Set("content-type", "application/json")

	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(configToRoute.BasicAuthConfig.Username + ":" + configToRoute.BasicAuthConfig.Password))
	httpReq.Header.Set("Authorization", "Basic "+encodedCredentials)

	client := &http.Client{}
	resp, err := client.Do(httpReq)

	if err != nil {
		zap.L().Error("Error encountered when executing request", zap.Any("request", requestBody), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return jsonrpc.ResponseBody{}, nil, err
	}
	defer resp.Body.Close()

	respBody, err := jsonrpc.DecodeResponseBody(resp)
	if err != nil {
		zap.L().Error("Could not deserialize response", zap.Any("request", requestBody), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return jsonrpc.ResponseBody{}, nil, err
	}

	zap.L().Debug("Successfully routed request to config.", zap.Any("request", requestBody), zap.Any("response", respBody), zap.Any("config", configToRoute))

	return respBody, resp, nil
}
