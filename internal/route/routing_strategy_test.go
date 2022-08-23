package route

import (
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
		assert.Equal(t, "something-else", strategy.routeNextRequest(upstreams))
		assert.Equal(t, "geth", strategy.routeNextRequest(upstreams))
	}
}

func TestPriorityStrategy_LowerPriority(t *testing.T) {
	upstreams := map[int][]string{
		0: {},
		1: {"fallback1", "fallback2"},
	}

	strategy := NewPriorityRoundRobinStrategy()

	for i := 0; i < 10; i++ {
		assert.Equal(t, "fallback2", strategy.routeNextRequest(upstreams))
		assert.Equal(t, "fallback1", strategy.routeNextRequest(upstreams))
	}
}

func TestPriorityStrategy_NoUpstreams(t *testing.T) {
	upstreams := map[int][]string{
		0: {},
		1: {},
	}

	strategy := NewPriorityRoundRobinStrategy()

	for i := 0; i < 10; i++ {
		assert.Equal(t, "", strategy.routeNextRequest(upstreams))
	}
}
