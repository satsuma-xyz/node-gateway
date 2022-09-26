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
	upstream := setUpHealthyUpstream(t)
	defer upstream.Close()

	upstreamConfigs := []config.UpstreamConfig{
		{ID: "testNode", HTTPURL: upstream.URL},
	}

	conf := config.Config{
		Upstreams: upstreamConfigs,
		Groups:    nil,
		Global:    config.GlobalConfig{},
	}

	router := wireRouter(conf)
	router.Start()

	for router.IsInitialized() == false {
		time.Sleep(10 * time.Millisecond)
	}
	handler := &RPCHandler{
		router: router,
	}

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

	responseBody, _ := jsonrpc.DecodeResponseBody(result)

	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, hexutil.Uint64(1000).String(), responseBody.Result)
}

func setUpHealthyUpstream(t *testing.T) *httptest.Server {
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
				Result: types.Header{Number: big.NewInt(1000)},
			}
			writeResponseBody(t, writer, body)

		case "eth_blockNumber":
			body := jsonrpc.ResponseBody{Result: hexutil.Uint64(1000)}
			writeResponseBody(t, writer, body)

		default:
			panic("Unknown method " + requestBody.Method)
		}
	}))
}

func writeResponseBody(t *testing.T, writer http.ResponseWriter, body jsonrpc.ResponseBody) {
	encodedBody, err := json.Marshal(body)
	assert.NoError(t, err)
	_, err = writer.Write(encodedBody)
	assert.NoError(t, err)
}
