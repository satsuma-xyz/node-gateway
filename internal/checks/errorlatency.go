package checks

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/failsafe-go/failsafe-go/circuitbreaker"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

const (
	ResponseCodeWildcard  = 'x'
	PercentPerFrac        = 100
	MinNumRequestsForRate = 1 // The minimum number of requests required to compute the error rate.
)

type ErrorCircuitBreaker interface {
	RecordResponse(isError bool)
	IsOpen() bool
}

type LatencyCircuitBreaker interface {
	RecordLatency(latency time.Duration)
	IsOpen() bool
	GetThreshold() time.Duration
}

type ErrorStats struct {
	circuitBreaker circuitbreaker.CircuitBreaker[any]
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

func NewErrorStats(routingConfig *conf.RoutingConfig) ErrorCircuitBreaker {
	return &ErrorStats{
		circuitBreaker: NewCircuitBreaker(
			getErrorsRate(routingConfig),
			getDetectionWindow(routingConfig),
			getBanWindow(routingConfig),
		),
	}
}

type LatencyStats struct {
	circuitBreaker circuitbreaker.CircuitBreaker[any]
	threshold      time.Duration
}

func (l *LatencyStats) RecordLatency(latency time.Duration) {
	if latency >= l.threshold {
		l.circuitBreaker.RecordFailure()
	} else {
		l.circuitBreaker.RecordSuccess()
	}
}

func (l *LatencyStats) IsOpen() bool {
	// TODO(polsar): We should be able to check `l.circuitBreaker.IsOpen()`,
	//  but it appears to remain open forever, regardless of the configured delay.
	//  We also must reset the circuit breaker manually if it is not supposed to be open.
	isOpen := l.circuitBreaker.RemainingDelay() > 0
	if !isOpen {
		l.circuitBreaker.Close()
	}

	return isOpen
}

func (l *LatencyStats) GetThreshold() time.Duration {
	return l.threshold
}

func NewLatencyStats(routingConfig *conf.RoutingConfig, method string) LatencyCircuitBreaker {
	return &LatencyStats{
		threshold: getLatencyThreshold(routingConfig, method),
		circuitBreaker: NewCircuitBreaker(
			conf.DefaultLatencyTooHighRate,
			getDetectionWindow(routingConfig),
			getBanWindow(routingConfig),
		),
	}
}

// NewCircuitBreaker abstracts away the rather complex circuitbreaker.Builder API.
// https://pkg.go.dev/github.com/failsafe-go/failsafe-go/circuitbreaker
// https://failsafe-go.dev/circuit-breaker/
func NewCircuitBreaker(
	errorRate float64,
	detectionWindow time.Duration,
	banWindow time.Duration,
) circuitbreaker.CircuitBreaker[any] {
	// TODO(polsar): Check that `0.0 < errorRate <= 1.0` holds.
	return circuitbreaker.Builder[any]().
		HandleResult(false). // The false return value of the wrapped call will be interpreted as a failure.
		WithFailureRateThreshold(
			uint(math.Floor(errorRate*PercentPerFrac)), // Minimum percentage of failed requests to open the breaker.
			MinNumRequestsForRate,
			detectionWindow,
		).
		WithDelay(banWindow).
		Build()
}

// ErrorLatencyCheck
// Error checking is disabled if `errorCircuitBreaker` is nil.
// Latency checking is disabled if `methodLatencyBreaker` is nil.
// At least one of these two must be non-nil.
type ErrorLatencyCheck struct {
	client               client.EthClient
	Err                  error
	clientGetter         client.EthClientGetter
	metricsContainer     *metrics.Container
	logger               *zap.Logger
	upstreamConfig       *conf.UpstreamConfig
	routingConfig        *conf.RoutingConfig
	errorCircuitBreaker  ErrorCircuitBreaker
	methodLatencyBreaker map[string]LatencyCircuitBreaker // RPC method -> LatencyCircuitBreaker
	lock                 sync.RWMutex
}

func NewErrorChecker(
	upstreamConfig *conf.UpstreamConfig,
	routingConfig *conf.RoutingConfig,
	clientGetter client.EthClientGetter,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) types.ErrorLatencyChecker {
	return NewErrorLatencyChecker(
		upstreamConfig,
		routingConfig,
		clientGetter,
		metricsContainer,
		logger,
		true,
		false,
	)
}

func NewLatencyChecker(
	upstreamConfig *conf.UpstreamConfig,
	routingConfig *conf.RoutingConfig,
	clientGetter client.EthClientGetter,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) types.ErrorLatencyChecker {
	return NewErrorLatencyChecker(
		upstreamConfig,
		routingConfig,
		clientGetter,
		metricsContainer,
		logger,
		false,
		true,
	)
}

func NewErrorLatencyChecker(
	upstreamConfig *conf.UpstreamConfig,
	routingConfig *conf.RoutingConfig,
	clientGetter client.EthClientGetter,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
	enableErrorChecking bool,
	enableLatencyChecking bool,
) types.ErrorLatencyChecker {
	if !enableErrorChecking && !enableLatencyChecking {
		panic("ErrorLatencyCheck must have at least one of error or latency checking enabled.")
	}

	// Create the error circuit breaker if error checking is enabled.
	var errorCircuitBreaker ErrorCircuitBreaker
	if enableErrorChecking {
		errorCircuitBreaker = NewErrorStats(routingConfig)
	}

	// Create the latency circuit breaker if latency checking is enabled.
	var latencyCircuitBreaker map[string]LatencyCircuitBreaker
	if enableLatencyChecking {
		latencyCircuitBreaker = make(map[string]LatencyCircuitBreaker)
	}

	return &ErrorLatencyCheck{
		upstreamConfig:       upstreamConfig,
		routingConfig:        routingConfig,
		clientGetter:         clientGetter,
		metricsContainer:     metricsContainer,
		logger:               logger,
		errorCircuitBreaker:  errorCircuitBreaker,
		methodLatencyBreaker: latencyCircuitBreaker,
	}
}

// Returns the LatencyStats instance corresponding to the specified RPC method.
// This method is thread-safe.
func (c *ErrorLatencyCheck) getLatencyCircuitBreaker(method string) LatencyCircuitBreaker {
	c.lock.Lock()
	defer c.lock.Unlock()

	stats, exists := c.methodLatencyBreaker[method]

	if !exists {
		// This is the first time we are checking this method so initialize its LatencyStats instance.
		stats = NewLatencyStats(c.routingConfig, method)
		c.methodLatencyBreaker[method] = stats
	}

	return stats
}

// IsPassing
// TODO(polsar): Split this method into two separate methods: IsPassingError and IsPassingLatency.
func (c *ErrorLatencyCheck) IsPassing(methods []string) bool {
	if !c.routingConfig.IsEnhancedRoutingControlDefined() {
		return true
	}

	if c.errorCircuitBreaker != nil && c.errorCircuitBreaker.IsOpen() {
		c.logger.Debug(
			"ErrorLatencyCheck is not passing due to too many errors.",
			zap.String("upstreamID", c.upstreamConfig.ID),
			zap.Error(c.Err),
		)

		return false
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.methodLatencyBreaker == nil {
		return true
	}

	// Only consider the passed methods, even if other methods' circuit breakers might be open.
	//
	// TODO(polsar): If the circuit breaker for any of the passed methods is open, we consider this upstream
	//  as unhealthy for all the other passed methods, even if their circuit breakers are closed. This might
	//  be undesirable, though since all the methods are part of the same request, we would have to somehow
	//  modify the request to only contain the healthy methods. This seems like more trouble than is worth.
	for _, method := range methods {
		if breaker, exists := c.methodLatencyBreaker[method]; exists && breaker.IsOpen() {
			c.logger.Debug(
				"ErrorLatencyCheck is not passing due to high latency of an RPC method.",
				zap.String("upstreamID", c.upstreamConfig.ID),
				zap.Any("method", method),
				zap.Error(c.Err),
			)

			return false
		}
	}

	return true
}

// RecordRequest
// TODO(polsar): Split this method into two separate methods: RecordError and RecordLatency.
func (c *ErrorLatencyCheck) RecordRequest(data *types.RequestData) {
	if !c.routingConfig.IsEnhancedRoutingControlDefined() {
		return
	}

	// Record the request latency if latency checking is enabled.
	if c.methodLatencyBreaker != nil {
		latencyCircuitBreaker := c.getLatencyCircuitBreaker(data.Method)
		latencyCircuitBreaker.RecordLatency(data.Latency)

		if data.Latency >= latencyCircuitBreaker.GetThreshold() {
			c.metricsContainer.ErrorLatencyCheckHighLatencies.WithLabelValues(
				c.upstreamConfig.ID,
				c.upstreamConfig.HTTPURL,
				metrics.HTTPRequest,
				data.Method,
			).Inc()
		}
	}

	// If error checking is disabled, we can return early.
	if c.errorCircuitBreaker == nil {
		return
	}

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

					// Even though this is a single HTTP request, we count each RPC JSON subresponse error.
					c.errorCircuitBreaker.RecordResponse(true) // JSON RPC subrequest error
				} else {
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
}

func (c *ErrorLatencyCheck) isError(httpCode, jsonRPCCode, errorMsg string) bool {
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

func getDetectionWindow(routingConfig *conf.RoutingConfig) time.Duration {
	if routingConfig != nil && routingConfig.DetectionWindow != nil {
		return *routingConfig.DetectionWindow
	}

	return conf.DefaultDetectionWindow
}

func getBanWindow(routingConfig *conf.RoutingConfig) time.Duration {
	if routingConfig != nil && routingConfig.BanWindow != nil {
		return *routingConfig.BanWindow
	}

	return conf.DefaultBanWindow
}

func getLatencyThreshold(routingConfig *conf.RoutingConfig, method string) time.Duration {
	if routingConfig != nil && routingConfig.Latency != nil {
		if latency, exists := routingConfig.Latency.MethodLatencyThresholds[method]; exists {
			return latency
		}

		return routingConfig.Latency.Threshold
	}

	return conf.DefaultMaxLatency
}

func getErrorsRate(routingConfig *conf.RoutingConfig) float64 {
	if routingConfig != nil && routingConfig.Errors != nil {
		return routingConfig.Errors.Rate
	}

	return conf.DefaultErrorRate
}
