package route

import (
	"errors"

	"sort"
	"sync/atomic"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/metadata"

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

type RequestMetadata struct{}

type NodeFilter interface {
	Apply(requestMetadata *RequestMetadata, upstreamID string) bool
}

type IsHealthyAndAtMaxHeightFilter struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsHealthyAndAtMaxHeightFilter) Apply(_ *RequestMetadata, upstreamID string) bool {
	var maxHeight = f.chainMetadataStore.GetMaxHeight()

	var upstreamStatus = f.healthCheckManager.GetUpstreamStatus(upstreamID)

	return upstreamStatus.IsHealthy(maxHeight)
}
