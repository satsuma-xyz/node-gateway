package route

import (
	"context"
	"encoding/json"

	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRouter_NoHealthyUpstreams(t *testing.T) {
	managerMock := mocks.NewHealthCheckManager(t)
	managerMock.EXPECT().StartHealthChecks()

	upstreamConfigs := []config.UpstreamConfig{
		{
			ID:      "geth",
			GroupID: "primary",
			HTTPURL: "gethURL",
		},
	}
	cacheConfig := config.ChainCacheConfig{}

	routingStrategy := mocks.NewMockRoutingStrategy(t)
	routingStrategy.EXPECT().RouteNextRequest(mock.Anything, mock.Anything).Return("", DefaultNoHealthyUpstreamsError)

	router := NewRouter("mainnet", cacheConfig, upstreamConfigs, make([]config.GroupConfig, 0), metadata.NewChainMetadataStore(), managerMock, routingStrategy, metrics.NewContainer(config.TestChainName), zap.L(), nil)
	router.(*SimpleRouter).healthCheckManager = managerMock
	router.Start()

	_, jsonResp, err := router.Route(context.Background(), &jsonrpc.BatchRequestBody{})

	assert.Nil(t, jsonResp)
	assert.Equal(t, DefaultNoHealthyUpstreamsError, err)
}

func TestRouter_GroupUpstreamsByPriority(t *testing.T) {
	managerMock := mocks.NewHealthCheckManager(t)

	httpClientMock := mocks.NewHTTPClient(t)
	httpResp := &http.Response{
		StatusCode: 203,
		Body:       io.NopCloser(strings.NewReader("{\"id\":1,\"jsonrpc\":\"2.0\",\"result\":\"hello\"}")),
	}
	httpClientMock.On("Do", mock.Anything).Return(httpResp, nil)

	routingStrategyMock := mocks.NewMockRoutingStrategy(t)
	routingStrategyMock.EXPECT().RouteNextRequest(mock.Anything, mock.Anything).Return("erigon", nil)

	gethConfig := config.UpstreamConfig{
		ID:      "geth",
		GroupID: "primary",
		HTTPURL: "gethURL",
	}
	erigonConfig := config.UpstreamConfig{
		ID:      "erigon",
		GroupID: "fallback",
		HTTPURL: "erigonURL",
	}
	openEthConfig := config.UpstreamConfig{
		ID:      "openethereum",
		GroupID: "backup",
		HTTPURL: "openEthURL",
	}
	somethingElseConfig := config.UpstreamConfig{
		ID:      "something-else",
		GroupID: "backup",
		HTTPURL: "something-elseURL",
	}
	configs := []config.UpstreamConfig{
		gethConfig,
		erigonConfig,
		openEthConfig,
		somethingElseConfig,
	}
	upstreamConfigs := configs

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
	cacheConfig := config.ChainCacheConfig{}

	router := NewRouter("mainnet", cacheConfig, upstreamConfigs, groupConfigs, metadata.NewChainMetadataStore(), managerMock, nil, metrics.NewContainer(config.TestChainName), zap.L(), nil)
	router.(*SimpleRouter).requestExecutor.httpClient = httpClientMock
	router.(*SimpleRouter).routingStrategy = routingStrategyMock

	upstreamID, jsonRPCResp, err := router.Route(context.Background(), &jsonrpc.SingleRequestBody{Method: "my_method"})

	assert.Nil(t, err)
	assert.Equal(t, erigonConfig.ID, upstreamID)
	assert.Equal(t, 203, httpResp.StatusCode)
	assert.Equal(t, json.RawMessage(`"hello"`), jsonRPCResp.(*jsonrpc.SingleResponseBody).Result)
	routingStrategyMock.AssertCalled(t, "RouteNextRequest", types.PriorityToUpstreamsMap{
		0: {&gethConfig},
		1: {&erigonConfig},
		2: {&openEthConfig, &somethingElseConfig},
	}, metadata.RequestMetadata{Methods: []string{"my_method"}})
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

	routingStrategyMock := mocks.NewMockRoutingStrategy(t)
	routingStrategyMock.EXPECT().RouteNextRequest(mock.Anything, mock.Anything).Return("erigon", nil)

	gethConfig := config.UpstreamConfig{
		ID:      "geth",
		HTTPURL: "gethURL",
	}
	erigonConfig := config.UpstreamConfig{
		ID:      "erigon",
		GroupID: "fallback",
		HTTPURL: "erigonURL",
	}
	upstreamConfigs := []config.UpstreamConfig{
		gethConfig,
		erigonConfig,
	}
	cacheConfig := config.ChainCacheConfig{}

	router := NewRouter("mainnet", cacheConfig, upstreamConfigs, make([]config.GroupConfig, 0), metadata.NewChainMetadataStore(), managerMock, nil, metrics.NewContainer(config.TestChainName), zap.L(), nil)
	router.(*SimpleRouter).requestExecutor.httpClient = httpClientMock
	router.(*SimpleRouter).routingStrategy = routingStrategyMock

	upstreamID, jsonRPCResp, err := router.Route(context.Background(), &jsonrpc.SingleRequestBody{Method: "my_method"})

	assert.Nil(t, err)
	assert.Equal(t, erigonConfig.ID, upstreamID)
	assert.Equal(t, 203, httpResp.StatusCode)
	assert.Equal(t, json.RawMessage(`"hello"`), jsonRPCResp.(*jsonrpc.SingleResponseBody).Result)
	routingStrategyMock.AssertCalled(t, "RouteNextRequest", types.PriorityToUpstreamsMap{
		0: {&gethConfig, &erigonConfig},
	}, metadata.RequestMetadata{Methods: []string{"my_method"}})
	assert.Equal(t, "erigonURL", httpClientMock.Calls[0].Arguments[0].(*http.Request).URL.Path)
}
