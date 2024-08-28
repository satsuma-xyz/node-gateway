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
	RecordRequest(isError bool)
	IsOpen() bool
}

type LatencyCircuitBreaker interface {
	RecordLatency(latency time.Duration)
	IsOpen() bool
	GetThreshold() time.Duration
}

type ErrorStats struct {
	slidingWindow SlidingWindow
	banWindow     time.Duration
	errorRate     float64
	lock          sync.RWMutex
}

func (e *ErrorStats) RecordRequest(isError bool) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.slidingWindow.AddValue(time.Duration(boolToInt(isError)))
}

func (e *ErrorStats) IsOpen() bool {
	e.lock.RLock()
	defer e.lock.RUnlock()

	return float64(e.slidingWindow.Sum().Nanoseconds())/float64(e.slidingWindow.Count()) >= e.errorRate
}

func NewErrorStats(routingConfig *conf.RoutingConfig) ErrorCircuitBreaker {
	return &ErrorStats{
		banWindow:     getBanWindow(routingConfig),
		errorRate:     getErrorsRate(routingConfig),
		slidingWindow: NewSimpleSlidingWindow(getDetectionWindow(routingConfig)),
	}
}

type LatencyStats struct {
	slidingWindow SlidingWindow
	banWindow     time.Duration
	threshold     time.Duration
	lock          sync.RWMutex
}

func (l *LatencyStats) RecordLatency(latency time.Duration) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.slidingWindow.AddValue(latency)
}

func (l *LatencyStats) IsOpen() bool {
	l.lock.RLock()
	defer l.lock.RUnlock()

	return l.slidingWindow.Mean() >= l.threshold
}

func (l *LatencyStats) GetThreshold() time.Duration {
	return l.threshold
}

func NewLatencyStats(routingConfig *conf.RoutingConfig, method string) LatencyCircuitBreaker {
	return &LatencyStats{
		banWindow:     getBanWindow(routingConfig),
		threshold:     getLatencyThreshold(routingConfig, method),
		slidingWindow: NewSimpleSlidingWindow(getDetectionWindow(routingConfig)),
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

	if err := c.InitializePassiveCheck(); err != nil {
		logger.Error("Error initializing LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig), zap.Error(err))
	}

	return c
}

func (c *LatencyCheck) InitializePassiveCheck() error {
	// TODO(polsar): Set `c.ShouldRun` if active health checking is enabled.
	c.logger.Debug("Initializing LatencyCheck.", zap.Any("config", c.upstreamConfig))

	httpClient, err := c.clientGetter(c.upstreamConfig.HTTPURL, &c.upstreamConfig.BasicAuthConfig, &c.upstreamConfig.RequestHeadersConfig)
	if err != nil {
		c.Err = err
		return c.Err
	}

	c.client = httpClient

	c.runPassiveCheck()

	// TODO(polsar): This check is in both PeerCheck and SyncingCheck implementations, so refactor this.
	if isMethodNotSupportedErr(c.Err) {
		c.logger.Debug("LatencyCheck is not supported by upstream, not running check.", zap.String("upstreamID", c.upstreamConfig.ID))

		c.ShouldRun = false
	}

	return nil
}

func (c *LatencyCheck) RunPassiveCheck() {
	if c.client == nil {
		if err := c.InitializePassiveCheck(); err != nil {
			c.logger.Error("Error initializing LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Error(err))
			c.metricsContainer.LatencyCheckErrors.WithLabelValues(c.upstreamConfig.ID, c.upstreamConfig.HTTPURL, metrics.HTTPInit).Inc()
		}
	}

	if c.ShouldRun {
		c.runPassiveCheck()
	}
}

func (c *LatencyCheck) runPassiveCheck() {
	if c.client == nil || !c.routingConfig.PassiveLatencyChecking {
		return
	}

	latencyConfig := c.routingConfig.Latency
	if latencyConfig == nil {
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
				c.runPassiveCheckForMethod(method, latencyThreshold)
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
		stats = NewLatencyStats(c.routingConfig, method)
		c.methodLatencyBreaker[method] = stats
	}

	return stats
}

// This method runs the passive latency check for the specified method and latency threshold.
func (c *LatencyCheck) runPassiveCheckForMethod(method string, latencyThreshold time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), RPCRequestTimeout)
	defer cancel()

	latencyBreaker := c.getLatencyCircuitBreaker(method)

	// Make and record the request.
	var duration time.Duration
	duration, c.Err = c.client.RecordLatency(ctx, method)
	// TODO(polsar): The error must also pass the checks specified in the config
	//  (i.e. match HTTP code, JSON RPC code, and error message).
	//  Fixing this is not a priority since we're not currently using passive health checking.
	isError := c.Err != nil
	c.errorCircuitBreaker.RecordRequest(isError)
	latencyBreaker.RecordLatency(duration)

	if isError {
		c.metricsContainer.LatencyCheckErrors.WithLabelValues(
			c.upstreamConfig.ID,
			c.upstreamConfig.HTTPURL,
			metrics.HTTPRequest,
		).Inc()
	} else if duration >= latencyThreshold {
		c.metricsContainer.LatencyCheckHighLatencies.WithLabelValues(
			c.upstreamConfig.ID,
			c.upstreamConfig.HTTPURL,
			metrics.HTTPRequest,
		).Inc()
	}

	c.metricsContainer.Latency.WithLabelValues(
		c.upstreamConfig.ID,
		c.upstreamConfig.HTTPURL,
	).Set(float64(duration.Milliseconds()))

	c.logger.Debug("Ran passive LatencyCheck.", zap.Any("upstreamID", c.upstreamConfig.ID), zap.Any("latency", duration), zap.Error(c.Err))
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

	// TODO(polsar): If one method's latency check is failing, the upstream will be marked as unhealthy,
	//  which will affect all other methods. We want to maintain the health status for each method separately.
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

func (c *LatencyCheck) RecordRequest(data *types.RequestData) {
	if c.routingConfig.PassiveLatencyChecking {
		return
	}

	latencyCircuitBreaker := c.getLatencyCircuitBreaker(data.Method)
	latencyCircuitBreaker.RecordLatency(data.Latency)

	if data.Latency >= latencyCircuitBreaker.GetThreshold() {
		c.metricsContainer.LatencyCheckHighLatencies.WithLabelValues(
			c.upstreamConfig.ID,
			c.upstreamConfig.HTTPURL,
			metrics.HTTPRequest,
		).Inc()
	}

	if data.HTTPResponseCode >= http.StatusBadRequest {
		// No RPC responses are available since the HTTP request errored out.
		// TODO(polsar): We might want to emit a Prometheus stat like we do for an RPC error below.
		c.errorCircuitBreaker.RecordRequest(c.isError(
			strconv.Itoa(data.HTTPResponseCode),
			"",
			"",
		)) // HTTP request error
	} else if data.ResponseBody != nil {
		for _, resp := range data.ResponseBody.GetSubResponses() {
			if resp.Error != nil {
				// Do not ignore this response even if it does not correspond to an RPC request.
				if c.isError("", strconv.Itoa(resp.Error.Code), resp.Error.Message) {
					c.metricsContainer.LatencyCheckErrors.WithLabelValues(
						c.upstreamConfig.ID,
						c.upstreamConfig.HTTPURL,
						metrics.HTTPRequest,
					).Inc()

					// Even though this is a single HTTP request, we count each RPC JSON subresponse error.
					c.errorCircuitBreaker.RecordRequest(true) // JSON RPC subrequest error
				} else {
					c.errorCircuitBreaker.RecordRequest(false) // JSON RPC subrequest OK
				}
			}
		}

		c.errorCircuitBreaker.RecordRequest(false) // HTTP request OK
	}
	// TODO(polsar): What does it mean when `data.ResponseBody == nil` and no HTTP error occurred?
	//  Log this strange case as an error.

	c.metricsContainer.Latency.WithLabelValues(
		c.upstreamConfig.ID,
		c.upstreamConfig.HTTPURL,
	).Set(float64(data.Latency.Milliseconds()))
}

func (c *LatencyCheck) isError(httpCode, jsonRPCCode, errorMsg string) bool {
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

func boolToInt(b bool) int {
	if b {
		return 1
	}

	return 0
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
	}

	return conf.DefaultMaxLatency
}

func getErrorsRate(routingConfig *conf.RoutingConfig) float64 {
	if routingConfig != nil && routingConfig.Errors != nil {
		return routingConfig.Errors.Rate
	}

	return conf.DefaultErrorRate
}
