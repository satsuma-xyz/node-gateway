package route

import (
	"errors"
	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/metadata"
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

var NoHealthyUpstreams = errors.New("No healthy upstreams")

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

	return "", NoHealthyUpstreams
}

type RequestMetadata struct{}

type NodeFilter interface {
	Apply(requestMetadata *RequestMetadata, upstreamId string) bool
}

type IsHealthyAndAtMaxHeightFilter struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsHealthyAndAtMaxHeightFilter) Apply(_ *RequestMetadata, upstreamId string) bool {
	var maxHeight = f.chainMetadataStore.GetMaxHeight()
	var upstreamStatus = f.healthCheckManager.GetUpstreamStatus(upstreamId)

	return upstreamStatus.IsHealthy(maxHeight)
}
