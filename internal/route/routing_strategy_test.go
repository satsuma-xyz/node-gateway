package route

import (
	"errors"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestPriorityStrategy_HighPriority(t *testing.T) {
	upstreams := types.PriorityToUpstreamsMap{
		0: {cfg("geth"), cfg("something-else")},
		1: {cfg("erigon")},
	}

	strategy := NewPriorityRoundRobinStrategy()

	for i := 0; i < 10; i++ {
		firstUpstreamID, _ := strategy.RouteNextRequest(upstreams)
		assert.Equal(t, "something-else", firstUpstreamID)

		secondUpstreamID, _ := strategy.RouteNextRequest(upstreams)
		assert.Equal(t, "geth", secondUpstreamID)
	}
}

func cfg(ID string) *config.UpstreamConfig {
	return &config.UpstreamConfig{
		ID: ID,
	}
}

func TestPriorityStrategy_LowerPriority(t *testing.T) {
	upstreams := types.PriorityToUpstreamsMap{
		0: {},
		1: {cfg("fallback1"), cfg("fallback2")},
	}

	strategy := NewPriorityRoundRobinStrategy()

	for i := 0; i < 10; i++ {
		firstUpstreamID, _ := strategy.RouteNextRequest(upstreams)
		assert.Equal(t, "fallback2", firstUpstreamID)

		secondUpstreamID, _ := strategy.RouteNextRequest(upstreams)
		assert.Equal(t, "fallback1", secondUpstreamID)
	}
}

func TestPriorityStrategy_NoUpstreams(t *testing.T) {
	upstreams := types.PriorityToUpstreamsMap{
		0: {},
		1: {},
	}

	strategy := NewPriorityRoundRobinStrategy()

	for i := 0; i < 10; i++ {
		upstreamID, err := strategy.RouteNextRequest(upstreams)
		assert.Equal(t, "", upstreamID)
		assert.True(t, errors.Is(err, ErrNoHealthyUpstreams))
	}
}
