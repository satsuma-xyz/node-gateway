package checks

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

const (
	ResponseCodeWildcard = 'x'
)

type ErrorCircuitBreaker interface {
	AddError()
	IsOpen() bool
}

type LatencyCircuitBreaker interface {
	AddLatency()
	IsOpen() bool
}

type ErrorStats struct {
	detectionWindow *time.Duration
	banWindow       *time.Duration
	errorsConfig    *conf.ErrorsConfig
	timeoutOrError  uint64
}

func (e *ErrorStats) AddError() {
	e.timeoutOrError++
}

func (e *ErrorStats) IsOpen() bool {
	return e.timeoutOrError > 0
}

func NewErrorStats(routingConfig *conf.RoutingConfig) ErrorCircuitBreaker {
	return &ErrorStats{
		detectionWindow: routingConfig.DetectionWindow,
		banWindow:       routingConfig.BanWindow,
		errorsConfig:    routingConfig.Errors,
		timeoutOrError:  0,
	}
}

type LatencyStats struct {
	detectionWindow *time.Duration
	banWindow       *time.Duration
	latencyConfig   *conf.LatencyConfig
	// TODO(polsar): Average out the latencies over the `detectionWindow`.
	// For V2, add the minimum number of measurements required to make a decision,
	// as well as other aggregation options.
	latencyTooHigh uint64
}

func (l *LatencyStats) AddLatency() {
	l.latencyTooHigh++
}

func (l *LatencyStats) IsOpen() bool {
	return l.latencyTooHigh > 0
}

func NewLatencyStats(routingConfig *conf.RoutingConfig) LatencyCircuitBreaker {
	return &LatencyStats{
		detectionWindow: routingConfig.DetectionWindow,
		banWindow:       routingConfig.BanWindow,
		latencyConfig:   routingConfig.Latency,
		latencyTooHigh:  0,
	}
}

type LatencyCheck struct {
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
	ShouldRun            bool
}

func NewLatencyChecker(
	upstreamConfig *conf.UpstreamConfig,
	routingConfig *conf.RoutingConfig,
	clientGetter client.EthClientGetter,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) types.LatencyChecker {
	c := &LatencyCheck{
		upstreamConfig:       upstreamConfig,
		routingConfig:        routingConfig,
		clientGetter:         clientGetter,
		metricsContainer:     metricsContainer,
		logger:               logger,
		errorCircuitBreaker:  NewErrorStats(routingConfig),
		methodLatencyBreaker: make(map[string]LatencyCircuitBreaker),
		ShouldRun:            routingConfig.Errors != nil || routingConfig.Latency != nil,
	}

	if err := c.Initialize(); err != nil {
		logger.Error("Error initializing LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Error(err))
	}

	return c
}

func (c *LatencyCheck) Initialize() error {
	c.logger.Debug("Initializing LatencyCheck.", zap.Any("config", c.upstreamConfig))

	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &c.upstreamConfig.BasicAuthConfig, &c.upstreamConfig.RequestHeadersConfig)
	if err != nil {
		c.Err = err
		return c.Err
	}

	c.client = httpClient

	c.runCheck()

	// TODO(polsar): This check is in both PeerCheck and SyncingCheck implementations, but I don't understand what it's supposed to be doing.
	// The setup is exactly the same in each case, so which method is not supported if the `isMethodNotSupportedErr` call returns `true`?
	if isMethodNotSupportedErr(c.Err) {
		c.logger.Debug("LatencyCheck is not supported by upstream, not running check.", zap.String("upstreamID", c.upstreamConfig.ID))

		c.ShouldRun = false
	}

	return nil
}

func (c *LatencyCheck) RunCheck() {
	if c.client == nil {
		if err := c.Initialize(); err != nil {
			c.logger.Error("Error initializing LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
			c.metricsContainer.LatencyCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPInit).Inc()
		}
	}

	if c.ShouldRun {
		c.runCheck()
	}
}

func (c *LatencyCheck) runCheck() {
	if c.client == nil || !c.routingConfig.PassiveLatencyChecking {
		return
	}

	latencyConfig := c.routingConfig.Latency
	if latencyConfig == nil {
		// TODO(polsar): We still want to check the latency of LatencyCheckMethod using the top-level latency threshold.
		return
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	// Iterate over all (method, latencyThreshold) pairs and launch the check for each in parallel.
	// Note that `latencyConfig.MethodLatencyThresholds` is never modified after its initialization
	// in `config` package, so we don't need a lock to protect concurrent read access.
	for method, latencyThreshold := range latencyConfig.MethodLatencyThresholds {
		wg.Add(1)

		// Passing the loop variables as arguments is required to prevent the following lint error:
		// loopclosure: loop variable method captured by func literal (govet)
		go func(method string, latencyThreshold time.Duration) {
			defer wg.Done()

			runCheck := func() {
				c.runCheckForMethod(method, latencyThreshold)
			}

			runCheckWithMetrics(runCheck,
				c.metricsContainer.LatencyCheckRequests.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL),
				c.metricsContainer.LatencyCheckDuration.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL))
		}(method, latencyThreshold)
	}
}

// Returns the LatencyStats instance corresponding to the specified RPC method.
// This method is thread-safe.
func (c *LatencyCheck) getLatencyCircuitBreaker(method string) LatencyCircuitBreaker {
	c.lock.Lock()
	defer c.lock.Unlock()

	stats, exists := c.methodLatencyBreaker[method]

	if !exists {
		// This is the first time we are checking this method so initialize its LatencyStats instance.
		//
		// TODO(polsar): Initialize all (method, LatencyStats) pairs in the Initialize method instead.
		// Once initialized, the map will only be read, eliminating the need for the lock.
		//
		// TODO(polsar): How do we want to keep track of methods that that don't have latency configuration?
		// Since the top-level latency threshold is used for all these methods, it probably makes sense to
		// keep track of all of them in the same LatencyStats instance. Note that this only applies if
		// PassiveLatencyChecking is false, since we would not know about and therefore could not check
		// these methods if PassiveLatencyChecking is true.
		stats = NewLatencyStats(c.routingConfig)
		c.methodLatencyBreaker[method] = stats
	}

	return stats
}

// This method runs the latency check for the specified method and latency threshold.
func (c *LatencyCheck) runCheckForMethod(method string, latencyThreshold time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), RPCRequestTimeout)
	defer cancel()

	latencyBreaker := c.getLatencyCircuitBreaker(method)

	// Make the request and increment the appropriate failure count if it takes too long or errors out.
	var duration time.Duration
	duration, c.Err = c.client.Latency(ctx, method)

	if c.Err != nil {
		c.errorCircuitBreaker.AddError()

		c.metricsContainer.LatencyCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
	} else if duration > latencyThreshold {
		latencyBreaker.AddLatency()

		c.metricsContainer.LatencyCheckHighLatencies.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPRequest).Inc()
	}

	c.metricsContainer.Latency.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL).Set(float64(duration.Milliseconds()))

	c.logger.Debug("Ran LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Any("latency", duration), zap.Error(c.Err))
}

func (c *LatencyCheck) IsPassing() bool {
	if c.errorCircuitBreaker.IsOpen() {
		c.logger.Debug(
			"LatencyCheck is not passing due to too many errors.",
			zap.String("upstreamID", c.upstreamConfig.ID),
			zap.Error(c.Err),
		)

		return false
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	// TODO(polsar): If one method's latency check is failing, the check will fail for all other methods. Is this what we want?
	for method, breaker := range c.methodLatencyBreaker {
		if breaker.IsOpen() {
			c.logger.Debug(
				"LatencyCheck is not passing due to high latency of an RPC method.",
				zap.String("upstreamID", c.upstreamConfig.ID),
				zap.Any("method", method),
				zap.Error(c.Err),
			)

			return false
		}
	}

	return true
}

func (c *LatencyCheck) recordError(method string) { //nolint:revive // Will be implemented soon
	// TODO(polsar): Implement this.
}

func (c *LatencyCheck) recordLatency(method string, latency time.Duration) { //nolint:revive // Will be implemented soon
	// TODO(polsar): Implement this.
}

func (c *LatencyCheck) RecordRequest(data *types.RequestData) {
	if c.routingConfig.PassiveLatencyChecking {
		return
	}

	c.recordLatency(data.Method, data.Latency)

	if data.HTTPResponseCode >= http.StatusBadRequest {
		// No RPC responses are available since the HTTP request errored out.
		c.processError(data.Method, strconv.Itoa(data.HTTPResponseCode), "", "")
		return
	}

	if data.ResponseBody == nil {
		// TODO(polsar): What does this even mean when no HTTP error occurred? How should we handle this case?
		return
	}

	for _, resp := range data.ResponseBody.GetSubResponses() {
		if resp.Error != nil {
			// TODO(polsar): Should we ignore this response if it does not correspond to an RPC request?
			c.processError(data.Method, "", strconv.Itoa(resp.Error.Code), resp.Error.Message)
		}
	}
}

func (c *LatencyCheck) processError(method, httpCode, jsonRPCCode, errorMsg string) {
	if isMatchForPatterns(httpCode, c.routingConfig.Errors.HTTPCodes) ||
		isMatchForPatterns(jsonRPCCode, c.routingConfig.Errors.JSONRPCCodes) ||
		isErrorMatches(errorMsg, c.routingConfig.Errors.ErrorStrings) {
		c.recordError(method)
	}
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
		// TODO(polsar): Unicode sucks. Fix this awkward conversion voodoo.
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
