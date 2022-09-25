package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/stretchr/testify/assert"
)

func TestSyncingThenNotSyncing(t *testing.T) {
	upstreamConfigs := []config.UpstreamConfig{
		{ID: "testNode"},
	}
	//chainMetadataStore := metadata.NewChainMetadataStore()
	//ethClientGetter := client.NewEthClient
	//healthCheckManager := checks.NewHealthCheckManager(ethClientGetter, upstreamConfigs, chainMetadataStore, time.NewTicker(checks.PeriodicHealthCheckInterval))
	//enabledNodeFilters := []route.NodeFilterType{route.IsHealthy, route.MaxHeightForGroup, route.SimpleIsStatePresent}
	//nodeFilter := route.CreateNodeFilter(enabledNodeFilters, healthCheckManager, chainMetadataStore)
	//routingStrategy := route.FilteringRoutingStrategy{
	//	NodeFilter:      nodeFilter,
	//	BackingStrategy: route.NewPriorityRoundRobinStrategy(),
	//}
	//router := route.NewRouter(upstreamConfigs, nil, chainMetadataStore, healthCheckManager, &routingStrategy)

	//router := mocks.NewRouter(t)
	undecodableContent := []byte("content")

	//router.On("Route", mock.Anything, mock.Anything).
	//	Return(nil, nil, jsonrpc.DecodeError{Err: errors.New("error decoding"), Content: undecodableContent})

	//handler := &RPCHandler{router: router}

	conf := config.Config{
		Upstreams: upstreamConfigs,
		Groups:    nil,
		Global:    config.GlobalConfig{},
	}

	router := wireRouter(conf)
	router.Start()
	handler := &RPCHandler{
		router: router,
	}
	//server := NewRPCServer(conf)
	//go func() {
	//	server.Start()
	//}()
	//handler := server.httpServer

	emptyJSONBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  nil,
		"id":      1,
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	_, _ = jsonrpc.DecodeResponseBody(result)

	assert.Equal(t, http.StatusOK, result.StatusCode)
	body, _ := io.ReadAll(result.Body)
	assert.Equal(t, undecodableContent, body)
}
