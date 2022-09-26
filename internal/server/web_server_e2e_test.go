package server

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/stretchr/testify/assert"
)

func TestServeHTTP_ForwardsToSoleHealthyUpstream(t *testing.T) {
	upstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, w http.ResponseWriter){})
	defer upstream.Close()

	upstreamConfigs := []config.UpstreamConfig{
		{ID: "testNode", HTTPURL: upstream.URL},
	}

	conf := config.Config{
		Upstreams: upstreamConfigs,
		Groups:    nil,
		Global:    config.GlobalConfig{},
	}

	handler := startRouterAndHandler(conf)

	result, responseBody := executeRequest(t, "eth_blockNumber", handler)

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, hexutil.Uint64(1000).String(), responseBody.Result)
}

func TestServeHTTP_ForwardsToArchiveNodeForStateRequest(t *testing.T) {
	getTransactionCount := "eth_getTransactionCount"
	expectedTransactionCount := 17

	fullNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, w http.ResponseWriter){
		getTransactionCount: func(t *testing.T, writer http.ResponseWriter) {
			t.Error("Unexpected call to stateful method on a full node!")
		},
	})
	defer fullNodeUpstream.Close()

	archiveNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, w http.ResponseWriter){
		getTransactionCount: func(t *testing.T, writer http.ResponseWriter) {
			t.Logf("Serving method %s from archive node as expected.", getTransactionCount)

			responseBody := jsonrpc.ResponseBody{Result: hexutil.Uint64(expectedTransactionCount)}
			writeResponseBody(t, writer, responseBody)
		},
	})
	defer archiveNodeUpstream.Close()

	fullNodeGroupID := "FullNodeGroup"
	archiveNodeGroupID := "ArchiveNodeGroup"

	upstreamConfigs := []config.UpstreamConfig{
		{ID: "testNodeFull", HTTPURL: fullNodeUpstream.URL, NodeType: config.Full, GroupID: fullNodeGroupID},
		{ID: "testNodeArchive", HTTPURL: archiveNodeUpstream.URL, NodeType: config.Archive, GroupID: archiveNodeGroupID},
	}

	// Force router to try full node with a higher priority to ensure filtering works.
	groupConfigs := []config.GroupConfig{
		{ID: fullNodeGroupID, Priority: 0},
		{ID: archiveNodeGroupID, Priority: 1},
	}
	conf := config.Config{
		Upstreams: upstreamConfigs,
		Groups:    groupConfigs,
		Global:    config.GlobalConfig{},
	}

	handler := startRouterAndHandler(conf)

	result, responseBody := executeRequest(t, getTransactionCount, handler)

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, hexutil.Uint64(expectedTransactionCount).String(), responseBody.Result)
}

func executeRequest(
	t *testing.T,
	methodName string,
	handler *RPCHandler,
) (*http.Response, *jsonrpc.ResponseBody) {
	emptyJSONBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  methodName,
		"params":  nil,
		"id":      1,
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(emptyJSONBody))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	responseBody, err := jsonrpc.DecodeResponseBody(result)
	assert.NoError(t, err)
	assert.NotNil(t, responseBody)
	return result, responseBody
}

func startRouterAndHandler(conf config.Config) *RPCHandler {
	router := wireRouter(conf)
	router.Start()

	for router.IsInitialized() == false {
		time.Sleep(10 * time.Millisecond)
	}
	handler := &RPCHandler{
		router: router,
	}
	return handler
}

func setUpHealthyUpstream(
	t *testing.T,
	additionalHandlers map[string]func(t *testing.T, w http.ResponseWriter),
) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestBody, err := jsonrpc.DecodeRequestBody(request)
		assert.NoError(t, err)

		switch requestBody.Method {
		case "eth_syncing":
			body := jsonrpc.ResponseBody{Result: false}
			writeResponseBody(t, writer, body)

		case "net_peerCount":
			body := jsonrpc.ResponseBody{Result: hexutil.Uint64(10)}
			writeResponseBody(t, writer, body)

		case "eth_getBlockByNumber":
			body := jsonrpc.ResponseBody{
				Result: types.Header{
					Number:     big.NewInt(1000),
					Difficulty: big.NewInt(0),
				},
			}
			writeResponseBody(t, writer, body)

		case "eth_blockNumber":
			body := jsonrpc.ResponseBody{Result: hexutil.Uint64(1000)}
			writeResponseBody(t, writer, body)

		default:
			if customHandler, found := additionalHandlers[requestBody.Method]; found {
				customHandler(t, writer)
			} else {
				panic("Unknown method " + requestBody.Method)
			}
		}
	}))
}

func writeResponseBody(t *testing.T, writer http.ResponseWriter, body jsonrpc.ResponseBody) {
	encodedBody, err := json.Marshal(body)
	assert.NoError(t, err)
	_, err = writer.Write(encodedBody)
	assert.NoError(t, err)
}
