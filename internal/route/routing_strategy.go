package route

import (
	"sort"
	"sync/atomic"

	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
)

//go:generate mockery --output ../mocks --name RoutingStrategy --structname MockRoutingStrategy --with-expecter
type RoutingStrategy interface {
	// RouteNextRequest returns the next UpstreamID a request should route to.
	RouteNextRequest(
		upstreamsByPriority types.PriorityToUpstreamsMap,
		requestMetadata metadata.RequestMetadata,
	) (string, error)
}
type PriorityRoundRobinStrategy struct {
	logger  *zap.Logger
	counter uint64
}

func NewPriorityRoundRobinStrategy(logger *zap.Logger) *PriorityRoundRobinStrategy {
	return &PriorityRoundRobinStrategy{
		logger:  logger,
		counter: 0,
	}
}

type NoHealthyUpstreamsError struct {
	msg string
}

var DefaultNoHealthyUpstreamsError = &NoHealthyUpstreamsError{"no healthy upstreams found"}

func (e *NoHealthyUpstreamsError) Error() string {
	return e.msg
}

func (s *PriorityRoundRobinStrategy) RouteNextRequest(
	upstreamsByPriority types.PriorityToUpstreamsMap,
	_ metadata.RequestMetadata,
) (string, error) {
	prioritySorted := maps.Keys(upstreamsByPriority)
	sort.Ints(prioritySorted)

	for _, priority := range prioritySorted {
		upstreams := upstreamsByPriority[priority]

		if len(upstreams) > 0 {
			atomic.AddUint64(&s.counter, 1)

			return upstreams[int(s.counter)%len(upstreams)].ID, nil
		}

		s.logger.Debug("Did not find any healthy nodes in priority.", zap.Int("priority", priority))
	}

	return "", DefaultNoHealthyUpstreamsError
}
