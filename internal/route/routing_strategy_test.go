package route

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPriorityStrategy_HighPriority(t *testing.T) {
	upstreams := map[int][]string{
		0: {"geth", "something-else"},
		1: {"erigon"},
	}

	strategy := NewPriorityRoundRobinStrategy()

	for i := 0; i < 10; i++ {
		firstUpstreamID, _ := strategy.RouteNextRequest(upstreams)
		assert.Equal(t, "something-else", firstUpstreamID)

		secondUpstreamID, _ := strategy.RouteNextRequest(upstreams)
		assert.Equal(t, "geth", secondUpstreamID)
	}
}

func TestPriorityStrategy_LowerPriority(t *testing.T) {
	upstreams := map[int][]string{
		0: {},
		1: {"fallback1", "fallback2"},
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
	upstreams := map[int][]string{
		0: {},
		1: {},
	}

	strategy := NewPriorityRoundRobinStrategy()

	for i := 0; i < 10; i++ {
		upstreamID, err := strategy.RouteNextRequest(upstreams)
		assert.Equal(t, "", upstreamID)
		assert.True(t, errors.Is(err, NoHealthyUpstreams))
	}
}
