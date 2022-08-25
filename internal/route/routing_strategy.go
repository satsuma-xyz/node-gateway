package route

import (
	"sort"
	"sync/atomic"

	"go.uber.org/zap"
	"golang.org/x/exp/maps"
)

//go:generate mockery --output ../mocks --name RoutingStrategy
type RoutingStrategy interface {
	// Returns the next UpstreamID a request should route to.
	RouteNextRequest(upstreamsByPriority map[int][]string) string
}
type PriorityRoundRobinStrategy struct {
	counter uint64
}

func NewPriorityRoundRobinStrategy() *PriorityRoundRobinStrategy {
	return &PriorityRoundRobinStrategy{
		counter: 0,
	}
}

func (s *PriorityRoundRobinStrategy) RouteNextRequest(upstreamsByPriority map[int][]string) string {
	prioritySorted := maps.Keys(upstreamsByPriority)
	sort.Ints(prioritySorted)

	for _, priority := range prioritySorted {
		upstreams := upstreamsByPriority[priority]

		if len(upstreams) > 0 {
			atomic.AddUint64(&s.counter, 1)

			return upstreams[int(s.counter)%len(upstreams)]
		}

		zap.L().Debug("Did not find any healthy nodes in priority.", zap.Int("priority", priority))
	}

	return ""
}
