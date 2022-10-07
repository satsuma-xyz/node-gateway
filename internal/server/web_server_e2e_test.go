package server

import (
	"bytes"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeHTTP_ForwardsToSoleHealthyUpstream(t *testing.T) {
	healthyUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody{})
	defer healthyUpstream.Close()

	unhealthyUpstream := setUpUnhealthyUpstream(t)
	defer unhealthyUpstream.Close()

	upstreamConfigs := []config.UpstreamConfig{
		{ID: "healthyNode", HTTPURL: healthyUpstream.URL},
		{ID: "unhealthyNode", HTTPURL: unhealthyUpstream.URL},
	}

	conf := config.Config{
		Upstreams: upstreamConfigs,
		Groups:    nil,
		Global:    config.GlobalConfig{},
	}

	handler := startRouterAndHandler(conf)

	statusCode, responseBody := executeRequest(t, []string{"eth_blockNumber"}, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, hexutil.Uint64(1000).String(), responseBody.Responses[0].Result)
}

func TestServeHTTP_ForwardsToCorrectNodeTypeBasedOnStatefulness(t *testing.T) {
	statefulMethod := "eth_getTransactionCount"
	expectedTransactionCount := 17

	nonStatefulMethod := "eth_getBlockTransactionCountByNumber"
	expectedBlockTxCount := 29

	fullNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody{
		//nolint:unparam // error method always return nil
		statefulMethod: func(t *testing.T, _ jsonrpc.RequestBody) *jsonrpc.ResponseBody {
			t.Helper()
			t.Errorf("Unexpected call to stateful method %s on a full node!", statefulMethod)

			return nil
		},
		nonStatefulMethod: func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody {
			t.Helper()
			t.Logf("Serving method %s from full node as expected", nonStatefulMethod)

			return &jsonrpc.ResponseBody{
				Result: hexutil.Uint64(expectedBlockTxCount),
				ID:     int(request.ID),
			}
		},
	})
	defer fullNodeUpstream.Close()

	archiveNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody{
		statefulMethod: func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody {
			t.Helper()
			t.Logf("Serving method %s from archive node as expected.", statefulMethod)

			return &jsonrpc.ResponseBody{
				Result: hexutil.Uint64(expectedTransactionCount),
				ID:     int(request.ID),
			}
		},
		//nolint:unparam // error method always return nil
		nonStatefulMethod: func(t *testing.T, _ jsonrpc.RequestBody) *jsonrpc.ResponseBody {
			t.Helper()
			t.Errorf("Unexpected call to method %s: archive node is at lower priority!", nonStatefulMethod)

			return nil
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

	statusCode, responseBody := executeRequest(t, []string{statefulMethod}, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, hexutil.Uint64(expectedTransactionCount).String(), responseBody.Responses[0].Result)

	statusCode, responseBody = executeRequest(t, []string{nonStatefulMethod}, handler)

	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, hexutil.Uint64(expectedBlockTxCount).String(), responseBody.Responses[0].Result)
}

func TestServeHTTP_ForwardsToCorrectNodeTypeBasedOnStatefulnessBatch(t *testing.T) {
	statefulMethod := "eth_getTransactionCount"
	expectedTransactionCount := 17

	nonStatefulMethod := "eth_getBlockTransactionCountByNumber"
	expectedBlockTxCount := 29

	fullNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody{
		//nolint:unparam // error method always return nil
		statefulMethod: func(t *testing.T, _ jsonrpc.RequestBody) *jsonrpc.ResponseBody {
			t.Helper()
			t.Errorf("Unexpected call to stateful method %s on a full node!", statefulMethod)

			return nil
		},
		//nolint:unparam // error method always return nil
		nonStatefulMethod: func(t *testing.T, _ jsonrpc.RequestBody) *jsonrpc.ResponseBody {
			t.Helper()
			t.Errorf("Unexpected call to non-stateful method %s on a full node!", nonStatefulMethod)

			return nil
		},
	})
	defer fullNodeUpstream.Close()

	archiveNodeUpstream := setUpHealthyUpstream(t, map[string]func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody{
		statefulMethod: func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody {
			t.Helper()
			t.Logf("Serving method %s from archive node as expected.", statefulMethod)

			return &jsonrpc.ResponseBody{
				Result: hexutil.Uint64(expectedTransactionCount),
				ID:     int(request.ID),
			}
		},
		nonStatefulMethod: func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody {
			t.Helper()
			t.Logf("Serving method %s from archive node as expected.", nonStatefulMethod)

			return &jsonrpc.ResponseBody{
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
	statusCode, responseBody := executeRequest(t, []string{statefulMethod, nonStatefulMethod}, handler)

	assert.Equal(t, http.StatusOK, statusCode)

	for _, response := range responseBody.Responses {
		switch response.ID {
		case 0: // statefulMethod
			assert.Equal(t, hexutil.Uint64(expectedTransactionCount).String(), response.Result)
		case 1: // nonStatefulMethod
			assert.Equal(t, hexutil.Uint64(expectedBlockTxCount).String(), response.Result)
		default:
			t.Errorf("Unexpected response with ID: %s encountered", strconv.Itoa(response.ID))
		}
	}
}

func executeRequest(t *testing.T, methodNames []string, handler *RPCHandler) (int, *jsonrpc.BatchResponseBody) {
	t.Helper()

	requests := make([]jsonrpc.RequestBody, 0)

	for i, methodName := range methodNames {
		singleRequest := jsonrpc.RequestBody{
			JSONRPCVersion: "2.0",
			Method:         methodName,
			ID:             int64(i),
		}
		requests = append(requests, singleRequest)
	}

	batchRequest := jsonrpc.BatchRequestBody{Requests: requests}
	batchRequestBytes, _ := batchRequest.EncodeRequestBody()

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(batchRequestBytes))
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
	additionalHandlers map[string]func(t *testing.T, request jsonrpc.RequestBody) *jsonrpc.ResponseBody,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		batchRequest, err := jsonrpc.DecodeRequestBody(request)
		assert.NoError(t, err)

		latestBlockNumber := int64(1000)

		responses := make([]jsonrpc.ResponseBody, 0)

		for _, request := range batchRequest.Requests {
			switch request.Method {
			case "eth_syncing":
				body := jsonrpc.ResponseBody{Result: false}
				responses = append(responses, body)

			case "net_peerCount":
				body := jsonrpc.ResponseBody{Result: hexutil.Uint64(10)}
				responses = append(responses, body)

			case "eth_getBlockByNumber":
				body := jsonrpc.ResponseBody{
					Result: types.Header{
						Number:     big.NewInt(latestBlockNumber),
						Difficulty: big.NewInt(0),
					},
				}
				responses = append(responses, body)

			case "eth_blockNumber":
				body := jsonrpc.ResponseBody{Result: hexutil.Uint64(latestBlockNumber)}
				responses = append(responses, body)

			default:
				if customHandler, found := additionalHandlers[request.Method]; found {
					body := customHandler(t, request)
					if body != nil {
						responses = append(responses, *body)
					}
				} else {
					panic("Unknown method " + request.Method)
				}
			}
		}

		batchResponseBody := jsonrpc.BatchResponseBody{Responses: responses}
		writeResponseBody(t, writer, batchResponseBody)
	}))
}

func setUpUnhealthyUpstream(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		batchRequest, err := jsonrpc.DecodeRequestBody(request)
		assert.NoError(t, err)

		responses := make([]jsonrpc.ResponseBody, 0)

		for _, request := range batchRequest.Requests {
			switch request.Method {
			case "eth_syncing", "net_peerCount", "eth_getBlockByNumber":
				errorBody := jsonrpc.ResponseBody{Error: &jsonrpc.Error{Message: "This is a failing fake node!"}}
				responses = append(responses, errorBody)
			default:
				t.Errorf("Expected unhealthy node to not receive any requests but got %s!", request.Method)
			}
		}

		batchResponseBody := jsonrpc.BatchResponseBody{Responses: responses}
		writeResponseBody(t, writer, batchResponseBody)
	}))
}

func writeResponseBody(t *testing.T, writer http.ResponseWriter, body jsonrpc.BatchResponseBody) {
	t.Helper()

	encodedBody, err := body.EncodeResponseBody()
	assert.NoError(t, err)
	_, err = writer.Write(encodedBody)
	assert.NoError(t, err)
}
