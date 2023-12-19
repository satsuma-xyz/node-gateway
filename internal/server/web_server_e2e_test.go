package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/samber/lo"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

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
			ChainName: config.TestChainName,
			Upstreams: upstreamConfigs,
			Groups:    nil,
		}},
		Global: config.GlobalConfig{},
	}

	handler := startRouterAndHandler(t, conf)

	statusCode, responseBody, _ := executeSingleRequest(t, config.TestChainName, "eth_blockNumber", handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, getResultFromString(hexutil.Uint64(1000).String()), responseBody.(*jsonrpc.SingleResponseBody).Result)
}

func TestServeHTTP_ForwardsToCorrectUpstreamForChainName(t *testing.T) {
	healthyUpstream1 := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{})
	defer healthyUpstream1.Close()

	healthyUpstream2 := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{})
	defer healthyUpstream2.Close()

	upstreamConfig1 := config.UpstreamConfig{ID: "healthyNode1", HTTPURL: healthyUpstream1.URL, NodeType: config.Full}
	upstreamConfig2 := config.UpstreamConfig{ID: "healthyNode2", HTTPURL: healthyUpstream2.URL, NodeType: config.Full}

	conf := config.Config{
		Chains: []config.SingleChainConfig{{
			ChainName: config.TestChainName,
			Upstreams: []config.UpstreamConfig{upstreamConfig1},
			Groups:    nil,
		}, {
			ChainName: "another_test_net",
			Upstreams: []config.UpstreamConfig{
				upstreamConfig2,
			},
			Groups: nil,
		}},
		Global: config.GlobalConfig{},
	}

	handler := startRouterAndHandler(t, conf)

	{
		statusCode, responseBody, headers := executeSingleRequest(t, config.TestChainName, "eth_blockNumber", handler)

		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, getResultFromString(hexutil.Uint64(1000).String()), responseBody.GetSubResponses()[0].Result)
		assert.Equal(t, upstreamConfig1.ID, headers.Get("X-Upstream-ID"))
	}

	{
		statusCode, responseBody, headers := executeSingleRequest(t, "another_test_net", "eth_blockNumber", handler)

		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, getResultFromString(hexutil.Uint64(1000).String()), responseBody.GetSubResponses()[0].Result)
		assert.Equal(t, upstreamConfig2.ID, headers.Get("X-Upstream-ID"))
	}
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
				Result: getResultFromString(hexutil.Uint64(expectedBlockTxCount).String()),
				ID:     *request.ID,
			}
		},
	})
	defer fullNodeUpstream.Close()

	archiveNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{
		statefulMethod: func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Logf("Serving method %s from archive node as expected.", statefulMethod)

			return jsonrpc.SingleResponseBody{
				Result: getResultFromString(hexutil.Uint64(expectedTransactionCount).String()),
				ID:     *request.ID,
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
			ChainName: config.TestChainName,
			Upstreams: upstreamConfigs,
			Groups:    groupConfigs,
		}},
		Global: config.GlobalConfig{},
	}

	handler := startRouterAndHandler(t, conf)

	statusCode, responseBody, _ := executeSingleRequest(t, config.TestChainName, statefulMethod, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, getResultFromString(hexutil.Uint64(expectedTransactionCount).String()), responseBody.(*jsonrpc.SingleResponseBody).Result)

	statusCode, responseBody, _ = executeSingleRequest(t, config.TestChainName, nonStatefulMethod, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, getResultFromString(hexutil.Uint64(expectedBlockTxCount).String()), responseBody.(*jsonrpc.SingleResponseBody).Result)
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
				Result: getResultFromString(hexutil.Uint64(expectedTransactionCount).String()),
				ID:     *request.ID,
			}
		},
		nonStatefulMethod: func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()
			t.Logf("Serving method %s from archive node as expected.", nonStatefulMethod)

			return jsonrpc.SingleResponseBody{
				Result: getResultFromString(hexutil.Uint64(expectedBlockTxCount).String()),
				ID:     *request.ID,
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
		Chains: []config.SingleChainConfig{{
			ChainName: config.TestChainName,
			Upstreams: upstreamConfigs,
			Groups:    groupConfigs,
		}},
		Global: config.GlobalConfig{},
	}

	handler := startRouterAndHandler(t, conf)

	// Batch request where one request in the batch is stateful. This should go to archive.
	statusCode, responseBody, _ := executeBatchRequest(t, config.TestChainName, []string{statefulMethod, nonStatefulMethod}, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, 2, len(responseBody.GetSubResponses()))

	idsToExpectedResult := make(map[int64]any)
	for _, response := range responseBody.GetSubResponses() {
		idsToExpectedResult[response.ID] = response.Result
	}

	assert.Equal(t, getResultFromString(hexutil.Uint64(expectedTransactionCount).String()), idsToExpectedResult[0])
	assert.Equal(t, getResultFromString(hexutil.Uint64(expectedBlockTxCount).String()), idsToExpectedResult[1])
}

func executeSingleRequest(
	t *testing.T,
	chainName string,
	methodName string,
	handler *http.ServeMux,
) (int, jsonrpc.ResponseBody, http.Header) {
	t.Helper()

	singleRequest := jsonrpc.SingleRequestBody{
		JSONRPCVersion: "2.0",
		Method:         methodName,
		ID:             lo.ToPtr[int64](1),
	}

	return executeRequest(t, chainName, &singleRequest, handler)
}

func executeBatchRequest(
	t *testing.T,
	chainName string,
	methodNames []string,
	handler *http.ServeMux,
) (int, jsonrpc.ResponseBody, http.Header) {
	t.Helper()

	requests := make([]jsonrpc.SingleRequestBody, 0)

	for i, methodName := range methodNames {
		singleRequest := jsonrpc.SingleRequestBody{
			JSONRPCVersion: "2.0",
			Method:         methodName,
			ID:             lo.ToPtr(int64(i)),
		}
		requests = append(requests, singleRequest)
	}

	batchRequest := jsonrpc.BatchRequestBody{Requests: requests}

	return executeRequest(t, chainName, &batchRequest, handler)
}

func executeRequest(
	t *testing.T,
	chainName string,
	request jsonrpc.RequestBody,
	handler *http.ServeMux,
) (int, jsonrpc.ResponseBody, http.Header) {
	t.Helper()

	requestBytes, _ := request.Encode()
	req := httptest.NewRequest(http.MethodPost, "/"+chainName, bytes.NewReader(requestBytes))

	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result()
	resultBody, _ := io.ReadAll(result.Body)

	defer result.Body.Close()

	responseBody, err := jsonrpc.DecodeResponseBody(resultBody)
	assert.NoError(t, err)
	require.NotNil(t, responseBody)

	return result.StatusCode, responseBody, recorder.Header()
}

func startRouterAndHandler(t *testing.T, conf config.Config) *http.ServeMux {
	t.Helper()

	err := conf.Validate()
	require.NoError(t, err)

	testLogger := zap.L()

	dependencyContainer := WireDependenciesForAllChains(conf, testLogger)

	dependencyContainer.RouterCollection.Start()

	for dependencyContainer.RouterCollection.IsInitialized() == false {
		time.Sleep(10 * time.Millisecond)
	}

	handler := dependencyContainer.Handler

	return handler
}

func setUpHealthyUpstream(
	t *testing.T,
	additionalHandlers map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestBodyRawBytes, err := io.ReadAll(request.Body)
		assert.NoError(t, err)

		requestBody, err := jsonrpc.DecodeRequestBody(requestBodyRawBytes)
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
		return jsonrpc.SingleResponseBody{Result: json.RawMessage(`false`)}

	case "net_peerCount":
		return jsonrpc.SingleResponseBody{Result: getResultFromString(hexutil.Uint64(10).String())}

	case "eth_getBlockByNumber":
		result, _ := json.Marshal(types.Header{
			Number:     big.NewInt(latestBlockNumber),
			Difficulty: big.NewInt(0),
		})

		return jsonrpc.SingleResponseBody{
			Result: json.RawMessage(result),
		}

	case "eth_blockNumber":
		return jsonrpc.SingleResponseBody{Result: getResultFromString(hexutil.Uint64(latestBlockNumber).String())}

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
		requestBodyRawBytes, err := io.ReadAll(request.Body)
		assert.NoError(t, err)

		requestBody, err := jsonrpc.DecodeRequestBody(requestBodyRawBytes)
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

func getResultFromString(input string) json.RawMessage {
	return json.RawMessage(`"` + input + `"`)
}
