package route

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"

	"net/http"

	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"go.uber.org/zap"
)

type RequestExecutor struct {
	httpClient client.HTTPClient
}

type ExecutorResult struct {
	err               error
	httpResponse      *http.Response
	batchResponseBody jsonrpc.BatchResponseBody
}

func (r *RequestExecutor) routeToConfig(
	ctx context.Context,
	batchRequest jsonrpc.BatchRequestBody,
	configToRoute *config.UpstreamConfig,
) (*jsonrpc.BatchResponseBody, *http.Response, error) {
	bodyBytes, err := batchRequest.EncodeRequestBody()
	if err != nil {
		zap.L().Error("Could not serialize request.", zap.Any("request", batchRequest), zap.Error(err))
		return nil, nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", configToRoute.HTTPURL, bytes.NewReader(bodyBytes))
	if err != nil {
		zap.L().Error("Could not create new http request.", zap.Any("request", batchRequest), zap.Error(err))
		return nil, nil, err
	}

	httpReq.Header.Set("content-type", "application/json")

	encodedCredentials := base64.StdEncoding.EncodeToString([]byte(configToRoute.BasicAuthConfig.Username + ":" + configToRoute.BasicAuthConfig.Password))
	httpReq.Header.Set("Authorization", "Basic "+encodedCredentials)

	resp, err := r.httpClient.Do(httpReq)

	if err != nil {
		zap.L().Error("Error encountered when executing request.", zap.Any("request", batchRequest), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, err
	}
	defer resp.Body.Close()

	respBody, err := jsonrpc.DecodeResponseBody(resp)
	if err != nil {
		zap.L().Warn("Could not deserialize response.", zap.Any("request", batchRequest), zap.String("response", fmt.Sprintf("%v", resp)), zap.Error(err))
		return nil, nil, err
	}

	zap.L().Debug("Successfully routed request to upstream.", zap.String("upstreamID", configToRoute.ID), zap.Any("request", batchRequest), zap.Any("response", respBody))

	return respBody, resp, nil
}
