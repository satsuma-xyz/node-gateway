package route

import (
	"sync/atomic"
)

type RoutingStrategy interface {
	// Returns the next UpstreamID a request should route to.
	routeNextRequest(upstreamIDs []string) string
}

type RoundRobinStrategy struct {
	counter uint64
}

func NewRoundRobinStrategy() *RoundRobinStrategy {
	return &RoundRobinStrategy{
		counter: 0,
	}
}

func (s *RoundRobinStrategy) routeNextRequest(upstreamIDs []string) string {
	// Don't worry about this overflowing. Wraps around to 0 past `math.MaxUint64`.
	atomic.AddUint64(&s.counter, 1)
	return upstreamIDs[int(s.counter)%len(upstreamIDs)]
}
