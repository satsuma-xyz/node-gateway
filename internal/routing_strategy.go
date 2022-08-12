package internal

import (
	"sort"
	"sync"
	"sync/atomic"
)

type RoutingStrategy interface {
	// Returns the next NodeID a request should route to.
	routeNextRequest() string
	updateNodeIDs([]string)
}

type RoundRobinStrategy struct {
	nodeIDMutex *sync.RWMutex
	nodeIDs     []string
	counter     uint64
}

func NewRoundRobinStrategy() *RoundRobinStrategy {
	return &RoundRobinStrategy{
		counter:     0,
		nodeIDs:     nil,
		nodeIDMutex: &sync.RWMutex{},
	}
}

func (s *RoundRobinStrategy) routeNextRequest() string {
	// Don't worry about this overflowing. Wraps around to 0 past `math.MaxUint64`.
	atomic.AddUint64(&s.counter, 1)

	s.nodeIDMutex.Lock()
	defer s.nodeIDMutex.Unlock()

	return s.nodeIDs[int(s.counter)%len(s.nodeIDs)]
}

func (s *RoundRobinStrategy) updateNodeIDs(nodeIDs []string) {
	s.nodeIDMutex.Lock()
	defer s.nodeIDMutex.Unlock()
	sort.Strings(nodeIDs)
	s.nodeIDs = nodeIDs
}
