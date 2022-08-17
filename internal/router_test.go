package internal

import (
	"io"
	"sync"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
)

func TestRouter_NoHealthyUpstreams(t *testing.T) {
	managerMock := mocks.NewHealthCheckManager(t)
	managerMock.On("GetHealthyUpstreams").Return([]string{})

	configs := []UpstreamConfig{
		{
			ID:                "mainnet",
			HTTPURL:           "http://rpc.ankr.io/eth",
			WSURL:             "wss://something/something",
			HealthCheckConfig: HealthCheckConfig{UseWSForBlockHeight: newBool(false)},
		},
	}

	router := SimpleRouter{
		healthCheckManager: managerMock,
		upstreamConfigs:    configs,
		upstreamsMutex:     &sync.RWMutex{},
		routingStrategy:    NewRoundRobinStrategy(),
	}

	jsonResp, httpResp, err := router.Route(jsonrpc.RequestBody{})
	defer httpResp.Body.Close()

	assert.Equal(t, jsonrpc.ResponseBody{}, jsonResp)
	assert.Equal(t, 503, httpResp.StatusCode)
	assert.Equal(t, "No healthy upstream", readyBody(httpResp.Body))
	assert.Nil(t, err)
}

func readyBody(body io.ReadCloser) string {
	bodyBytes, _ := io.ReadAll(body)
	return string(bodyBytes)
}
