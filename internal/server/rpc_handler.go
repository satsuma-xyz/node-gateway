package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/route"
	"github.com/satsuma-data/node-gateway/internal/util"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

const defaultReadHeaderTimeout = 10 * time.Second

type RPCHandler struct {
	router route.Router
	logger *zap.Logger
	path   string
}

func (h *RPCHandler) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.URL.Path != h.path {
		panic(fmt.Sprintf("Unexpected request with path %s to handler for path %s!", req.URL.Path, h.path))
	}

	if req.Method != http.MethodPost {
		respondJSON(h.logger, writer, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	ctx := util.NewContext(context.Background(), getClientID(req))

	headerContentType := req.Header.Get("Content-Type")
	// Content-Type SHOULD be 'application/json-rpc' but MAY be
	// 'application/json' or 'application/jsonrequest'.
	// See https://www.jsonrpc.org/historical/json-rpc-over-http.html.
	if !slices.Contains([]string{"application/json", "application/json-rpc", "application/jsonrequest"}, headerContentType) {
		respondJSON(h.logger, writer, "Content-Type not supported.", http.StatusUnsupportedMediaType)
		return
	}

	// No need to close the request body, the Server implementation will take care of it.
	requestBodyRawBytes, err := io.ReadAll(req.Body)

	if err != nil {
		errMsg := fmt.Sprintf("Request body could not be read, err: %s", err.Error())
		h.logger.Error(errMsg)
		respondJSON(h.logger, writer, errMsg, http.StatusInternalServerError)

		return
	}

	requestBody, err := jsonrpc.DecodeRequestBody(requestBodyRawBytes)
	if err != nil {
		errMsg := fmt.Sprintf("Request body could not be parsed, err: %s", err.Error())
		resp := jsonrpc.CreateErrorJSONRPCResponseBody(errMsg, jsonrpc.InternalServerErrorCode)
		h.logger.Error(errMsg)
		respondJSONRPC(h.logger, writer, resp, http.StatusBadRequest)

		return
	}

	h.logger.Debug("Request received.", zap.String("method", req.Method), zap.String("path", req.URL.Path), zap.String("query", req.URL.RawQuery), zap.Any("body", requestBody))

	upstreamID, jsonRPCRespBody, err := h.router.Route(ctx, requestBody)

	if err != nil {
		switch e := err.(type) {
		// Still pass the response to client if we're not able to decode response from upstream.
		case *jsonrpc.DecodeError:
			respondRaw(nil, writer, e.Content, http.StatusOK)
			return
		case *route.NoHealthyUpstreamsError:
			respondJSON(h.logger, writer, "No healthy upstreams.", http.StatusServiceUnavailable)
			return
		default:
			statusCode := http.StatusInternalServerError
			if originErr, ok := err.(*route.OriginError); ok && originErr.ResponseCode != 0 {
				statusCode = originErr.ResponseCode
			}

			respondJSON(h.logger, writer, fmt.Sprintf("Request could not be routed, err: %s", err.Error()), statusCode)

			return
		}
	}

	writer.Header().Set("X-Upstream-ID", upstreamID)
	respondJSONRPC(h.logger, writer, jsonRPCRespBody, http.StatusOK)

	h.logger.Debug("Request successfully routed.", zap.Any("requestBody", requestBody))
}

func getClientID(req *http.Request) string {
	// Try to get the id of the `client` via query parameter. Admittedly this is a little hacky, but it won't break
	// functionality as query params aren't used in JSON RPC.
	// The reason we're using query param is because client code may be hard to modify, e.g. graph-nodes, while using
	// query param is part of the RPC URL which is usually supplied as a config to the client.
	// A better solution is to pass the client via an HTTP header.
	if clientID := req.URL.Query().Get("client"); clientID != "" {
		return clientID
	}

	return "unknown"
}

func respondJSONRPC(
	logger *zap.Logger,
	writer http.ResponseWriter,
	response jsonrpc.ResponseBody,
	httpStatusCode int,
) {
	if response == nil {
		writer.WriteHeader(httpStatusCode)
		return
	}

	respBytes, err := response.Encode()
	if err != nil {
		logger.Error("Failed to serialize response.", zap.Error(err), zap.String("response", string(respBytes)))
		return
	}

	writer.Header().Set("Content-Type", "application/json")

	// Note: Call `WriteHeader` last otherwise headers won't get written.
	// See Header() on the http.ResponseWriter interface for more information.
	writer.WriteHeader(httpStatusCode)

	if i, err := writer.Write(respBytes); err != nil {
		logger.Error("Failed to write JSON RPC response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(respBytes)))
		return
	}
}

func respondJSON(logger *zap.Logger, writer http.ResponseWriter, message string, httpStatusCode int) {
	resp := make(map[string]string)
	if message != "" {
		resp["message"] = message
	}

	writer.Header().Set("Content-Type", "application/json")

	// Note: Call `WriteHeader` last otherwise headers won't get written.
	// See Header() on the http.ResponseWriter interface for more information.
	writer.WriteHeader(httpStatusCode)

	jsonResp, _ := json.Marshal(resp)
	if i, err := writer.Write(jsonResp); err != nil {
		logger.Error("Failed to write response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(jsonResp)))
		return
	}
}

func respondRaw(logger *zap.Logger, writer http.ResponseWriter, body []byte, httpStatusCode int) {
	writer.WriteHeader(httpStatusCode)

	if i, err := writer.Write(body); err != nil {
		logger.Error("Failed to write raw response body.", zap.Error(err), zap.Int("bytesWritten", i), zap.String("response", string(body)))
		return
	}
}
