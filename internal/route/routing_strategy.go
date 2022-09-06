package route

import (
	"errors"

	"sort"
	"sync/atomic"

	"go.uber.org/zap"
	"golang.org/x/exp/maps"
)

//go:generate mockery --output ../mocks --name RoutingStrategy --with-expecter
type RoutingStrategy interface {
	// Returns the next UpstreamID a request should route to.
	RouteNextRequest(upstreamsByPriority map[int][]string) (string, error)
}
type PriorityRoundRobinStrategy struct {
	counter uint64
}

func NewPriorityRoundRobinStrategy() *PriorityRoundRobinStrategy {
	return &PriorityRoundRobinStrategy{
		counter: 0,
	}
}

var ErrNoHealthyUpstreams = errors.New("no healthy upstreams")

func (s *PriorityRoundRobinStrategy) RouteNextRequest(upstreamsByPriority map[int][]string) (string, error) {
	prioritySorted := maps.Keys(upstreamsByPriority)
	sort.Ints(prioritySorted)

	for _, priority := range prioritySorted {
		upstreams := upstreamsByPriority[priority]

		if len(upstreams) > 0 {
			atomic.AddUint64(&s.counter, 1)

			return upstreams[int(s.counter)%len(upstreams)], nil
		}

		zap.L().Debug("Did not find any healthy nodes in priority.", zap.Int("priority", priority))
	}

	return "", ErrNoHealthyUpstreams
}
