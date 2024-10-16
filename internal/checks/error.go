package checks

import (
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
)

const ResponseCodeWildcard = 'x'

type ErrorCheck struct {
	Err                 error
	metricsContainer    *metrics.Container
	logger              *zap.Logger
	upstreamConfig      *config.UpstreamConfig
	routingConfig       *config.RoutingConfig
	errorCircuitBreaker ErrorCircuitBreaker
	isCheckEnabled      bool
}

type ErrorCircuitBreaker interface {
	RecordResponse(isError bool)
	IsOpen() bool
}

type ErrorStats struct {
	circuitBreaker circuitbreaker.CircuitBreaker[any]
}

func NewErrorStats(routingConfig *config.RoutingConfig) ErrorCircuitBreaker {
	return &ErrorStats{
		circuitBreaker: NewCircuitBreaker(
			getErrorsRate(routingConfig),
			getDetectionWindow(routingConfig),
			getBanWindow(routingConfig),
		),
	}
}

func NewErrorChecker(
	upstreamConfig *config.UpstreamConfig,
	routingConfig *config.RoutingConfig,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) types.ErrorLatencyChecker {
	return &ErrorCheck{
		upstreamConfig:      upstreamConfig,
		routingConfig:       routingConfig,
		metricsContainer:    metricsContainer,
		logger:              logger,
		errorCircuitBreaker: NewErrorStats(routingConfig),
		isCheckEnabled:      routingConfig.IsEnabled,
	}
}

func (c *ErrorCheck) isError(httpCode, jsonRPCCode, errorMsg string) bool {
	if isMatchForPatterns(httpCode, c.routingConfig.Errors.HTTPCodes) ||
		isMatchForPatterns(jsonRPCCode, c.routingConfig.Errors.JSONRPCCodes) ||
		isErrorMatches(errorMsg, c.routingConfig.Errors.ErrorStrings) {
		return true
	}

	return false
}

func isMatchForPatterns(responseCode string, patterns []string) bool {
	if responseCode == "" {
		return false
	}

	if len(patterns) == 0 {
		return true
	}

	for _, pattern := range patterns {
		if isMatch(responseCode, pattern) {
			return true
		}
	}

	return false
}

// Returns true iff the response code matches the pattern using ResponseCodeWildcard as the wildcard character.
func isMatch(responseCode, pattern string) bool {
	if len(responseCode) != len(pattern) {
		return false
	}

	for i, x := range responseCode {
		y := string(pattern[i])

		if strings.EqualFold(y, string(ResponseCodeWildcard)) {
			continue
		}

		if string(x) != y {
			return false
		}
	}

	return true
}

func isErrorMatches(errorMsg string, errors []string) bool {
	if errorMsg == "" {
		return false
	}

	if len(errors) == 0 {
		return true
	}

	for _, errorSubstr := range errors {
		// TODO(polsar): Add support for regular expression matching.
		if strings.Contains(errorMsg, errorSubstr) {
			return true
		}
	}

	return false
}

func getErrorsRate(routingConfig *config.RoutingConfig) float64 {
	if routingConfig != nil && routingConfig.Errors != nil {
		return routingConfig.Errors.Rate
	}

	return config.DefaultErrorRate
}

func (e *ErrorStats) RecordResponse(isError bool) {
	if isError {
		e.circuitBreaker.RecordFailure()
	} else {
		e.circuitBreaker.RecordSuccess()
	}
}

func (e *ErrorStats) IsOpen() bool {
	// TODO(polsar): We should be able to check `e.circuitBreaker.IsOpen()`,
	//  but it appears to remain open forever, regardless of the configured delay.
	//  We also must reset the circuit breaker manually if it is not supposed to be open.
	isOpen := e.circuitBreaker.RemainingDelay() > 0
	if !isOpen {
		e.circuitBreaker.Close()
	}

	return isOpen
}

func (c *ErrorCheck) IsPassing([]string) bool {
	if !c.isCheckEnabled {
		return true
	}

	if c.errorCircuitBreaker != nil && c.errorCircuitBreaker.IsOpen() {
		c.logger.Debug(
			"ErrorCheck is not passing due to too many errors.",
			zap.String("upstreamID", c.upstreamConfig.ID),
			zap.Error(c.Err),
		)

		return false
	}

	return true
}

// RecordRequest records the request data for error checking. It returns true if we recorded an error.
// Note that a request may have an error which we do not record, in which case this method returns false.
func (c *ErrorCheck) RecordRequest(data *types.RequestData) bool {
	if !c.isCheckEnabled {
		return false
	}

	isError := false

	errorString := ""
	if data.Error != nil {
		errorString = data.Error.Error()
	}

	if data.HTTPResponseCode >= http.StatusBadRequest || data.ResponseBody == nil {
		// No RPC responses are available since the HTTP request errored out or does not contain a JSON RPC response.
		// TODO(polsar): We might want to emit a Prometheus stat like we do for an RPC error below.
		c.errorCircuitBreaker.RecordResponse(c.isError(
			strconv.Itoa(data.HTTPResponseCode), // Note that this CAN be 200 OK.
			"",
			errorString,
		))

		isError = true
	} else { // data.ResponseBody != nil
		for _, resp := range data.ResponseBody.GetSubResponses() {
			if resp.Error != nil {
				// Do not ignore this response even if it does not correspond to an RPC request.
				if c.isError("", strconv.Itoa(resp.Error.Code), resp.Error.Message) {
					c.metricsContainer.ErrorLatencyCheckErrors.WithLabelValues(
						c.upstreamConfig.ID,
						c.upstreamConfig.HTTPURL,
						metrics.HTTPRequest,
						data.Method,
					).Inc()

					isError = true

					// Even though this is a single HTTP request, we count each RPC JSON subresponse error.
					c.errorCircuitBreaker.RecordResponse(true) // JSON RPC subrequest error
				} else {
					c.metricsContainer.ErrorLatencyCheckNoErrors.WithLabelValues(
						c.upstreamConfig.ID,
						c.upstreamConfig.HTTPURL,
						metrics.HTTPRequest,
						data.Method,
					).Inc()

					c.errorCircuitBreaker.RecordResponse(false) // JSON RPC subrequest OK
				}
			}
		}
	}

	c.metricsContainer.ErrorLatency.WithLabelValues(
		c.upstreamConfig.ID,
		c.upstreamConfig.HTTPURL,
		data.Method,
	).Set(float64(data.Latency.Milliseconds()))

	return isError
}
