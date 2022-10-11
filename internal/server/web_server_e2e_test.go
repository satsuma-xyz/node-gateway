package server

import (
	"bytes"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testChainName = "test_net"

func TestMain(m *testing.M) {
	loggingConfig := zap.NewDevelopmentConfig()
	loggingConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	logger, err := loggingConfig.Build()
	if err != nil {
		panic(err.Error())
	}
	zap.ReplaceGlobals(logger)

	os.Exit(m.Run())
}

func TestServeHTTP_ForwardsToSoleHealthyUpstream(t *testing.T) {
	healthyUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{})
	defer healthyUpstream.Close()

	unhealthyUpstream := setUpUnhealthyUpstream(t)
	defer unhealthyUpstream.Close()

	upstreamConfigs := []config.UpstreamConfig{
		{ID: "healthyNode", HTTPURL: healthyUpstream.URL, NodeType: config.Full},
		{ID: "unhealthyNode", HTTPURL: unhealthyUpstream.URL, NodeType: config.Full},
	}

	conf := config.Config{
		Chains: []config.SingleChainConfig{{
			ChainName: testChainName,
			Upstreams: upstreamConfigs,
			Groups:    nil,
		}},
		Global: config.GlobalConfig{},
	}

	handler := startRouterAndHandler(t, conf)

	statusCode, responseBody := executeSingleRequest(t, "eth_blockNumber", handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, hexutil.Uint64(1000).String(), responseBody.(*jsonrpc.SingleResponseBody).Result)
}

func TestServeHTTP_ForwardsToCorrectNodeTypeBasedOnStatefulness(t *testing.T) {
	statefulMethod := "eth_getTransactionCount"
	expectedTransactionCount := 17

	nonStatefulMethod := "eth_getBlockTransactionCountByNumber"
	expectedBlockTxCount := 29

	fullNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{
		statefulMethod: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Errorf("Unexpected call to stateful method %s on a full node!", statefulMethod)

			return jsonrpc.SingleResponseBody{}
		},
		nonStatefulMethod: func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Logf("Serving method %s from full node as expected", nonStatefulMethod)

			return jsonrpc.SingleResponseBody{
				Result: hexutil.Uint64(expectedBlockTxCount),
				ID:     int(request.ID),
			}
		},
	})
	defer fullNodeUpstream.Close()

	archiveNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{
		statefulMethod: func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Logf("Serving method %s from archive node as expected.", statefulMethod)

			return jsonrpc.SingleResponseBody{
				Result: hexutil.Uint64(expectedTransactionCount),
				ID:     int(request.ID),
			}
		},
		nonStatefulMethod: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Errorf("Unexpected call to method %s: archive node is at lower priority!", nonStatefulMethod)

			return jsonrpc.SingleResponseBody{}
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
		Chains: []config.SingleChainConfig{{
			ChainName: testChainName,
			Upstreams: upstreamConfigs,
			Groups:    groupConfigs,
		}},
		Global: config.GlobalConfig{},
	}

	handler := startRouterAndHandler(t, conf)

	statusCode, responseBody := executeSingleRequest(t, statefulMethod, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, hexutil.Uint64(expectedTransactionCount).String(), responseBody.(*jsonrpc.SingleResponseBody).Result)

	statusCode, responseBody = executeSingleRequest(t, nonStatefulMethod, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, hexutil.Uint64(expectedBlockTxCount).String(), responseBody.(*jsonrpc.SingleResponseBody).Result)
}

func TestServeHTTP_ForwardsToCorrectNodeTypeBasedOnStatefulnessBatch(t *testing.T) {
	statefulMethod := "eth_getTransactionCount"
	expectedTransactionCount := 17

	nonStatefulMethod := "eth_getBlockTransactionCountByNumber"
	expectedBlockTxCount := 29

	fullNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{
		statefulMethod: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Errorf("Unexpected call to stateful method %s on a full node!", statefulMethod)

			return jsonrpc.SingleResponseBody{}
		},
		nonStatefulMethod: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Errorf("Unexpected call to non-stateful method %s on a full node!", nonStatefulMethod)

			return jsonrpc.SingleResponseBody{}
		},
	})
	defer fullNodeUpstream.Close()

	archiveNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{
		statefulMethod: func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Logf("Serving method %s from archive node as expected.", statefulMethod)

			return jsonrpc.SingleResponseBody{
				Result: hexutil.Uint64(expectedTransactionCount),
				ID:     int(request.ID),
			}
		},
		nonStatefulMethod: func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Logf("Serving method %s from archive node as expected.", nonStatefulMethod)

			return jsonrpc.SingleResponseBody{
				Result: hexutil.Uint64(expectedBlockTxCount),
				ID:     int(request.ID),
			}
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

	// Batch request where one request in the batch is stateful. This should go to archive.
	statusCode, responseBody := executeBatchRequest(t, []string{statefulMethod, nonStatefulMethod}, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, 2, len(responseBody.GetSubResponses()))

	idsToExpectedResult := make(map[int]any)
	for _, response := range responseBody.GetSubResponses() {
		idsToExpectedResult[response.ID] = response.Result
	}

	assert.Equal(t, hexutil.Uint64(expectedTransactionCount).String(), idsToExpectedResult[0])
	assert.Equal(t, hexutil.Uint64(expectedBlockTxCount).String(), idsToExpectedResult[1])
}

func executeSingleRequest(t *testing.T, methodName string, handler *RPCHandler) (int, jsonrpc.ResponseBody) {
	t.Helper()

	singleRequest := jsonrpc.SingleRequestBody{
		JSONRPCVersion: "2.0",
		Method:         methodName,
		ID:             1,
	}

	return executeRequest(t, &singleRequest, handler)
}

func executeBatchRequest(t *testing.T, methodNames []string, handler *RPCHandler) (int, jsonrpc.ResponseBody) {
	t.Helper()

	requests := make([]jsonrpc.SingleRequestBody, 0)

	for i, methodName := range methodNames {
		singleRequest := jsonrpc.SingleRequestBody{
			JSONRPCVersion: "2.0",
			Method:         methodName,
			ID:             int64(i),
		}
		requests = append(requests, singleRequest)
	}

	batchRequest := jsonrpc.BatchRequestBody{Requests: requests}

	return executeRequest(t, &batchRequest, handler)
}

func executeRequest(t *testing.T, request jsonrpc.RequestBody, handler *RPCHandler) (int, jsonrpc.ResponseBody) {
	t.Helper()

	requestBytes, _ := request.Encode()

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(requestBytes))
	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	defer result.Body.Close()

	responseBody, err := jsonrpc.DecodeResponseBody(result)
	assert.NoError(t, err)
	require.NotNil(t, responseBody)

	return result.StatusCode, responseBody
}

func startRouterAndHandler(t *testing.T, conf config.Config) *RPCHandler {
	t.Helper()

	err := conf.Validate()
	require.NoError(t, err)

	testLogger := zap.L()
	currentChainConfig := &conf.Chains[0]
	dependencyContainer := wireSingleChainDependencies(currentChainConfig, testLogger)
	router := dependencyContainer.router
	router.Start()

	for router.IsInitialized() == false {
		time.Sleep(10 * time.Millisecond)
	}

	handler := &RPCHandler{
		router: router,
		logger: testLogger,
	}

	return handler
}

func setUpHealthyUpstream(
	t *testing.T,
	additionalHandlers map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestBody, err := jsonrpc.DecodeRequestBody(request)
		assert.NoError(t, err)

		var responseBody jsonrpc.ResponseBody

		switch r := requestBody.(type) {
		case *jsonrpc.SingleRequestBody:
			singleResponse := handleSingleRequest(t, *r, additionalHandlers)
			responseBody = &singleResponse
		case *jsonrpc.BatchRequestBody:
			responses := make([]jsonrpc.SingleResponseBody, 0)

			for _, singleRequest := range r.GetSubRequests() {
				responses = append(responses, handleSingleRequest(t, singleRequest, additionalHandlers))
			}

			responseBody = &jsonrpc.BatchResponseBody{Responses: responses}
		default:
			panic(fmt.Sprintf("Invalid type: %s found for request.", reflect.TypeOf(r)))
		}

		writeResponseBody(t, writer, responseBody)
	}))
}

func handleSingleRequest(t *testing.T, request jsonrpc.SingleRequestBody,
	additionalHandlers map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody,
) jsonrpc.SingleResponseBody {
	t.Helper()

	latestBlockNumber := int64(1000)

	switch request.Method {
	case "eth_syncing":
		return jsonrpc.SingleResponseBody{Result: false}

	case "net_peerCount":
		return jsonrpc.SingleResponseBody{Result: hexutil.Uint64(10)}

	case "eth_getBlockByNumber":
		return jsonrpc.SingleResponseBody{
			Result: types.Header{
				Number:     big.NewInt(latestBlockNumber),
				Difficulty: big.NewInt(0),
			},
		}

	case "eth_blockNumber":
		return jsonrpc.SingleResponseBody{Result: hexutil.Uint64(latestBlockNumber)}

	default:
		if customHandler, found := additionalHandlers[request.Method]; found {
			return customHandler(t, request)
		}

		panic("Unknown method " + request.Method)
	}
}

func setUpUnhealthyUpstream(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestBody, err := jsonrpc.DecodeRequestBody(request)
		assert.NoError(t, err)

		var responseBody jsonrpc.ResponseBody

		switch r := requestBody.(type) {
		case *jsonrpc.SingleRequestBody:
			switch requestBody.GetMethod() {
			case "eth_syncing", "net_peerCount", "eth_getBlockByNumber":
				responseBody = &jsonrpc.SingleResponseBody{Error: &jsonrpc.Error{Message: "This is a failing fake node!"}}
				writeResponseBody(t, writer, responseBody)
			default:
				t.Errorf("Expected unhealthy node to not receive any requests but got %s!", request.Method)
			}
		case *jsonrpc.BatchRequestBody:
			t.Errorf("Expected unhealthy node to not receive any requests but got %s!", request.Method)
		default:
			panic(fmt.Sprintf("Invalid type: %s found for request.", reflect.TypeOf(r)))
		}
	}))
}

func writeResponseBody(t *testing.T, writer http.ResponseWriter, body jsonrpc.ResponseBody) {
	t.Helper()

	encodedBody, err := body.Encode()
	assert.NoError(t, err)
	_, err = writer.Write(encodedBody)
	assert.NoError(t, err)
}
