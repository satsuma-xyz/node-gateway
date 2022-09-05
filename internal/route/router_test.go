package route

import (
	"context"
	"github.com/satsuma-data/node-gateway/internal/types"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRouter_NoHealthyUpstreams(t *testing.T) {
	managerMock := mocks.NewHealthCheckManager(t)

	checkerMock := mocks.NewChecker(t)
	checkerMock.EXPECT().IsPassing().Return(false)
	managerMock.EXPECT().GetUpstreamStatus(mock.Anything).Return(&types.UpstreamStatus{
		PeerCheck:    checkerMock,
		SyncingCheck: checkerMock,
	})

	upstreamConfigs := []config.UpstreamConfig{
		{
			ID:      "geth",
			GroupID: "primary",
			HTTPURL: "gethURL",
		},
	}

	router := NewRouter(upstreamConfigs, make([]config.GroupConfig, 0), make(chan uint64), managerMock)
	router.(*SimpleRouter).healthCheckManager = managerMock

	jsonResp, httpResp, err := router.Route(context.Background(), jsonrpc.RequestBody{})
	defer httpResp.Body.Close()

	assert.Nil(t, jsonResp)
	assert.Equal(t, 503, httpResp.StatusCode)
	assert.Equal(t, "No healthy upstreams", readyBody(httpResp.Body))
	assert.Nil(t, err)
}

func readyBody(body io.ReadCloser) string {
	bodyBytes, _ := io.ReadAll(body)
	return string(bodyBytes)
}

func TestRouter_GroupUpstreamsByPriority(t *testing.T) {
	managerMock := mocks.NewHealthCheckManager(t)

	httpClientMock := mocks.NewHTTPClient(t)
	httpResp := &http.Response{
		StatusCode: 203,
		Body:       io.NopCloser(strings.NewReader("{\"id\":1,\"jsonrpc\":\"2.0\",\"result\":\"hello\"}")),
	}
	httpClientMock.On("Do", mock.Anything).Return(httpResp, nil)

	routingStrategyMock := mocks.NewRoutingStrategy(t)
	routingStrategyMock.EXPECT().RouteNextRequest(mock.Anything).Return("erigon", nil)

	upstreamConfigs := []config.UpstreamConfig{
		{
			ID:      "geth",
			GroupID: "primary",
			HTTPURL: "gethURL",
		},
		{
			ID:      "erigon",
			GroupID: "fallback",
			HTTPURL: "erigonURL",
		},
		{
			ID:      "openethereum",
			GroupID: "backup",
			HTTPURL: "openEthURL",
		},
		{
			ID:      "something-else",
			GroupID: "backup",
			HTTPURL: "something-elseURL",
		},
	}

	groupConfigs := []config.GroupConfig{
		{
			ID:       "primary",
			Priority: 0,
		},
		{
			ID:       "fallback",
			Priority: 1,
		},
		{
			ID:       "backup",
			Priority: 2,
		},
	}
	router := NewRouter(upstreamConfigs, groupConfigs, make(chan uint64), managerMock)
	router.(*SimpleRouter).requestExecutor.httpClient = httpClientMock
	router.(*SimpleRouter).routingStrategy = routingStrategyMock

	jsonRcpResp, httpResp, err := router.Route(context.Background(), jsonrpc.RequestBody{})
	defer httpResp.Body.Close()

	assert.Nil(t, err)
	assert.Equal(t, 203, httpResp.StatusCode)
	assert.NotNil(t, "hello", jsonRcpResp.Result)
	routingStrategyMock.AssertCalled(t, "RouteNextRequest", map[int][]string{
		0: {"geth"},
		1: {"erigon"},
		2: {"openethereum", "something-else"},
	})
	assert.Equal(t, "erigonURL", httpClientMock.Calls[0].Arguments[0].(*http.Request).URL.Path)
}

func TestGroupUpstreamsByPriority_NoGroups(t *testing.T) {
	managerMock := mocks.NewHealthCheckManager(t)

	httpClientMock := mocks.NewHTTPClient(t)
	httpResp := &http.Response{
		StatusCode: 203,
		Body:       io.NopCloser(strings.NewReader("{\"id\":1,\"jsonrpc\":\"2.0\",\"result\":\"hello\"}")),
	}
	httpClientMock.On("Do", mock.Anything).Return(httpResp, nil)

	routingStrategyMock := mocks.NewRoutingStrategy(t)
	routingStrategyMock.EXPECT().RouteNextRequest(mock.Anything).Return("erigon", nil)

	upstreamConfigs := []config.UpstreamConfig{
		{
			ID:      "geth",
			HTTPURL: "gethURL",
		},
		{
			ID:      "erigon",
			GroupID: "fallback",
			HTTPURL: "erigonURL",
		},
	}

	router := NewRouter(upstreamConfigs, make([]config.GroupConfig, 0), make(chan uint64), managerMock)
	router.(*SimpleRouter).requestExecutor.httpClient = httpClientMock
	router.(*SimpleRouter).routingStrategy = routingStrategyMock

	jsonRcpResp, httpResp, err := router.Route(context.Background(), jsonrpc.RequestBody{})
	defer httpResp.Body.Close()

	assert.Nil(t, err)
	assert.Equal(t, 203, httpResp.StatusCode)
	assert.NotNil(t, "hello", jsonRcpResp.Result)
	routingStrategyMock.AssertCalled(t, "RouteNextRequest", map[int][]string{
		0: {"geth", "erigon"},
	})
	assert.Equal(t, "erigonURL", httpClientMock.Calls[0].Arguments[0].(*http.Request).URL.Path)
}
