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
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/samber/lo"
	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var defaultRoutingConfig = config.RoutingConfig{
	DetectionWindow: config.NewDuration(10 * time.Minute),
	BanWindow:       config.NewDuration(100 * time.Millisecond),
	Errors: &config.ErrorsConfig{
		Rate: 0.5,
		HTTPCodes: []string{
			"5xx",
			"420",
		},
		JSONRPCCodes: []string{
			"32xxx",
		},
		ErrorStrings: []string{
			"internal server error",
		},
	},
	Latency: &config.LatencyConfig{
		MethodLatencyThresholds: map[string]time.Duration{
			"eth_call":    10000 * time.Millisecond,
			"eth_getLogs": 2000 * time.Millisecond,
		},
		Threshold: 1000 * time.Millisecond,
		Methods: []config.MethodConfig{
			{
				Name:      "eth_getLogs",
				Threshold: 2000 * time.Millisecond,
			},
			{
				Name:      "eth_call",
				Threshold: 10000 * time.Millisecond,
			},
		},
	},
}

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

	statusCode, responseBody, _, _ := executeSingleRequest(t, config.TestChainName, "eth_blockNumber", handler, false)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, getResultFromString(hexutil.Uint64(1000).String()), responseBody.(*jsonrpc.SingleResponseBody).Result)
}

func getHandler(t *testing.T, methodName, errMsg string) (*http.ServeMux, []*httptest.Server) {
	t.Helper()

	unhealthyUpstream := setUpUnhealthyUpstream(t)

	numCalls := 0
	healthyUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{
		methodName: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody {
			t.Helper()

			numCalls++
			if numCalls <= checks.MinNumRequestsForRate {
				// Even if we return en error on the first MinNumRequestsForRate calls,
				// the upstream will still be considered healthy.
				return jsonrpc.SingleResponseBody{
					Error: &jsonrpc.Error{
						Message: errMsg,
					}}
			}

			return jsonrpc.SingleResponseBody{
				Result: getResultFromString(hexutil.Uint64(1000).String()),
			}
		},
	})

	upstreamConfigs := []config.UpstreamConfig{
		{ID: "unhealthyNode", HTTPURL: unhealthyUpstream.URL, NodeType: config.Full},
		{ID: "healthyNode", HTTPURL: healthyUpstream.URL, NodeType: config.Full},
	}

	conf := config.Config{
		Chains: []config.SingleChainConfig{{
			ChainName: config.TestChainName,
			Upstreams: upstreamConfigs,
			Groups:    nil,
			Routing:   defaultRoutingConfig,
		}},
	}

	return startRouterAndHandler(t, conf), []*httptest.Server{healthyUpstream, unhealthyUpstream}
}

func TestServeHTTP_ForwardsToSoleHealthyUpstream_RoutingControlEnabled_ErrorStringDoesNotMatch(t *testing.T) {
	methodName := "eth_getLogs"
	errMsg := "This is a failing fake node!"
	handler, upstreams := getHandler(t, methodName, errMsg)

	defer func() {
		for _, upstream := range upstreams {
			upstream.Close()
		}
	}()

	for i := 0; i < checks.MinNumRequestsForRate; i++ {
		statusCode, responseBody, _, _ := executeSingleRequest(
			t,
			config.TestChainName,
			methodName,
			handler,
			false,
		)

		assert.Equal(t, http.StatusOK, statusCode)

		res, err := responseBody.(*jsonrpc.SingleResponseBody)

		assert.True(t, err)
		assert.Equal(t, 0, len(res.Result))
		assert.Equal(t, errMsg, res.Error.Message)
	}

	statusCode, responseBody, _, _ := executeSingleRequest(t, config.TestChainName, methodName, handler, false)

	// Even though the error rate exceeds the configured amount, the upstream is still considered healthy since
	// the error message does not match the configured error string.
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, getResultFromString(hexutil.Uint64(1000).String()), responseBody.(*jsonrpc.SingleResponseBody).Result)
}

func TestServeHTTP_ForwardsToSoleHealthyUpstream_RoutingControlEnabled_ErrorStringMatches(t *testing.T) {
	methodName := "eth_getLogs"
	errMsg := "Terrible internal server error occurred!"
	handler, upstreams := getHandler(t, methodName, errMsg)

	defer func() {
		for _, upstream := range upstreams {
			upstream.Close()
		}
	}()

	for i := 0; i < checks.MinNumRequestsForRate; i++ {
		statusCode, _, _, _ := executeSingleRequest(t, config.TestChainName, methodName, handler, false)

		assert.Equal(t, http.StatusOK, statusCode)
	}

	// The error rate exceeds the configured amount of 0.5, so the upstream is considered unhealthy.
	// Since `alwaysRoute` is disabled, we expect nil response on the next MinNumRequestsForRate requests.
	for i := 0; i < checks.MinNumRequestsForRate; i++ {
		statusCode, responseBody, _, err := executeSingleRequest(t, config.TestChainName, methodName, handler, true)

		assert.NotNil(t, err)
		assert.True(t, strings.Contains(err.Error(), "No healthy upstreams"))
		assert.Equal(t, http.StatusServiceUnavailable, statusCode)
		assert.Nil(t, responseBody)
	}

	// We now have to wait for the ban window to expire. We add a slight delay to avoid clock skew.
	time.Sleep(*defaultRoutingConfig.BanWindow + 10*time.Millisecond)

	statusCode, responseBody, _, _ := executeSingleRequest(t, config.TestChainName, methodName, handler, true)

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
		statusCode, responseBody, headers, _ := executeSingleRequest(t, config.TestChainName, "eth_blockNumber", handler, false)

		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, getResultFromString(hexutil.Uint64(1000).String()), responseBody.GetSubResponses()[0].Result)
		assert.Equal(t, upstreamConfig1.ID, headers.Get("X-Upstream-ID"))
	}

	{
		statusCode, responseBody, headers, _ := executeSingleRequest(t, "another_test_net", "eth_blockNumber", handler, false)

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
		statefulMethod: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody { //nolint:unparam // test method always returns err
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
		nonStatefulMethod: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody { //nolint:unparam // test method always returns err
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

	statusCode, responseBody, _, _ := executeSingleRequest(t, config.TestChainName, statefulMethod, handler, false)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, getResultFromString(hexutil.Uint64(expectedTransactionCount).String()), responseBody.(*jsonrpc.SingleResponseBody).Result)

	statusCode, responseBody, _, _ = executeSingleRequest(t, config.TestChainName, nonStatefulMethod, handler, false)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, getResultFromString(hexutil.Uint64(expectedBlockTxCount).String()), responseBody.(*jsonrpc.SingleResponseBody).Result)
}

func TestServeHTTP_ForwardsToCorrectNodeTypeBasedOnStatefulnessBatch(t *testing.T) {
	statefulMethod := "eth_getTransactionCount"
	expectedTransactionCount := 17

	nonStatefulMethod := "eth_getBlockTransactionCountByNumber"
	expectedBlockTxCount := 29

	fullNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody{
		statefulMethod: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody { //nolint:unparam // test method always returns err
			t.Helper()
			t.Errorf("Unexpected call to stateful method %s on a full node!", statefulMethod)

			return jsonrpc.SingleResponseBody{}
		},
		nonStatefulMethod: func(t *testing.T, _ jsonrpc.SingleRequestBody) jsonrpc.SingleResponseBody { //nolint:unparam // test method always returns err
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
	statusCode, responseBody, _, _ := executeBatchRequest(t, config.TestChainName, []string{statefulMethod, nonStatefulMethod}, handler)

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
	allowNilResponse bool,
) (int, jsonrpc.ResponseBody, http.Header, error) {
	t.Helper()

	singleRequest := jsonrpc.SingleRequestBody{
		JSONRPCVersion: "2.0",
		Method:         methodName,
		ID:             lo.ToPtr[int64](1),
	}

	return executeRequest(t, chainName, &singleRequest, handler, allowNilResponse)
}

func executeBatchRequest(
	t *testing.T,
	chainName string,
	methodNames []string,
	handler *http.ServeMux,
) (int, jsonrpc.ResponseBody, http.Header, error) {
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

	return executeRequest(t, chainName, &batchRequest, handler, false)
}

func executeRequest(
	t *testing.T,
	chainName string,
	request jsonrpc.RequestBody,
	handler *http.ServeMux,
	allowNilResponse bool,
) (int, jsonrpc.ResponseBody, http.Header, error) {
	t.Helper()

	requestBytes, _ := request.Encode()
	req := httptest.NewRequest(http.MethodPost, "/"+chainName, bytes.NewReader(requestBytes))

	req.Header.Add("Content-Type", "application/json")

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	result := recorder.Result() //nolint:bodyclose // Body is closed in the defer statement below.
	resultBody, _ := io.ReadAll(result.Body)

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil { //nolint:errorlint // Wrong error.
			t.Errorf("Error closing response body: %s", err)
		}
	}(result.Body)

	responseBody, err := jsonrpc.DecodeResponseBody(resultBody)

	// TODO(polsar): These checks should be done by the individual tests. Refactor.
	if !allowNilResponse {
		assert.NoError(t, err)
		require.NotNil(t, responseBody)
	}

	return result.StatusCode, responseBody, recorder.Header(), err
}

func startRouterAndHandler(t *testing.T, conf config.Config) *http.ServeMux { //nolint:gocritic // Legacy
	t.Helper()

	err := conf.Validate()
	require.NoError(t, err)

	testLogger := zap.L()

	dependencyContainer := WireDependenciesForAllChains(conf, testLogger)

	dependencyContainer.RouterCollection.Start()

	// Poll until the router is initialized.
	for !dependencyContainer.RouterCollection.IsInitialized() {
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

	case "eth_chainId":
		return jsonrpc.SingleResponseBody{Result: getResultFromString(hexutil.Uint64(11).String())}

	case "eth_getBlockByNumber":
		result, _ := json.Marshal(types.Header{
			Number:     big.NewInt(latestBlockNumber),
			Difficulty: big.NewInt(0),
		})

		return jsonrpc.SingleResponseBody{
			Result: result,
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
			case "eth_syncing", "net_peerCount", "eth_chainId", "eth_getBlockByNumber":
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
