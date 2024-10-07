package checks

import (
	"math"
	"time"

	"github.com/failsafe-go/failsafe-go/circuitbreaker"
	"github.com/satsuma-data/node-gateway/internal/config"
)

const (
	PercentPerFrac        = 100
	MinNumRequestsForRate = 1 // The minimum number of requests required to compute the error rate.
)

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

func getDetectionWindow(routingConfig *config.RoutingConfig) time.Duration {
	if routingConfig != nil && routingConfig.DetectionWindow != nil {
		return *routingConfig.DetectionWindow
	}

	return config.DefaultDetectionWindow
}

func getBanWindow(routingConfig *config.RoutingConfig) time.Duration {
	if routingConfig != nil && routingConfig.BanWindow != nil {
		return *routingConfig.BanWindow
	}

	return config.DefaultBanWindow
}
