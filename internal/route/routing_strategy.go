package route

import (
	"slices"
	"sort"
	"sync/atomic"

	conf "github.com/satsuma-data/node-gateway/internal/config"
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
	logger      *zap.Logger
	counter     uint64
	alwaysRoute bool
}

func NewPriorityRoundRobinStrategy(logger *zap.Logger, alwaysRoute bool) *PriorityRoundRobinStrategy {
	return &PriorityRoundRobinStrategy{
		logger:      logger,
		counter:     0,
		alwaysRoute: alwaysRoute,
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
	statusToUpstreamsByPriority := partitionUpstreams(upstreamsByPriority)

	var healthyUpstreamsByPriority types.PriorityToUpstreamsMap

	var exists bool

	if healthyUpstreamsByPriority, exists = statusToUpstreamsByPriority[conf.ReasonUnknownOrHealthy]; !exists {
		// There are no healthy upstreams.
		healthyUpstreamsByPriority = make(types.PriorityToUpstreamsMap)
	}

	prioritySorted := maps.Keys(healthyUpstreamsByPriority)
	sort.Ints(prioritySorted)

	// Note that `prioritySorted` can be empty, in which case the body of this loop will not be executed even once.
	for _, priority := range prioritySorted {
		upstreams := healthyUpstreamsByPriority[priority]

		if len(upstreams) > 0 {
			atomic.AddUint64(&s.counter, 1)

			return upstreams[int(s.counter)%len(upstreams)].ID, nil //nolint:nolintlint,gosec // Legacy
		}

		s.logger.Debug("Did not find any healthy nodes in priority.", zap.Int("priority", priority))
	}

	// No healthy upstreams are available. If `alwaysRoute` is true, find an unhealthy upstream to route to anyway.
	// TODO(polsar): At this time, the only unhealthy upstreams that can end up here are those due to high latency
	//  or error rate. Pass ALL configured upstreams in `upstreamsByPriority`. Their health status should be indicated
	//  in the UpstreamConfig.HealthStatus field.
	if s.alwaysRoute {
		s.logger.Warn("No healthy upstreams found but `alwaysRoute` is set to true.")

		// If available, return an upstream that's unhealthy due to high latency rate.
		if upstreamsByPriorityLatencyUnhealthy, ok := statusToUpstreamsByPriority[conf.ReasonLatencyTooHighRate]; ok {
			upstream := getHighestPriorityUpstream(upstreamsByPriorityLatencyUnhealthy)
			if upstream == nil {
				// This indicates a non-recoverable bug in the code.
				panic("Upstream not found!")
			}

			s.logger.Info(
				"Routing to an upstream with high latency.",
				zap.String("ID", upstream.ID),
				zap.String("GroupID", upstream.GroupID),
				zap.String("HTTPURL", upstream.HTTPURL),
				zap.String("WSURL", upstream.WSURL),
			)

			return upstream.ID, nil
		}

		// If available, return an upstream that's unhealthy due to high error rate.
		if upstreamsByPriorityErrorUnhealthy, ok := statusToUpstreamsByPriority[conf.ReasonErrorRate]; ok {
			upstream := getHighestPriorityUpstream(upstreamsByPriorityErrorUnhealthy)
			if upstream == nil {
				// This indicates a non-recoverable bug in the code.
				panic("Upstream not found!")
			}

			s.logger.Info(
				"Routing to an upstream with high error rate.",
				zap.String("ID", upstream.ID),
				zap.String("GroupID", upstream.GroupID),
				zap.String("HTTPURL", upstream.HTTPURL),
				zap.String("WSURL", upstream.WSURL),
			)

			return upstream.ID, nil
		}

		// TODO(polsar): If we get here, that means all upstreams are unhealthy, but they are all unhealthy
		//  due to a reason other than high latency or error rate. We should still be able to route to one of those.
		//  Asana task: https://app.asana.com/0/1207397277805097/1208186611173034/f
		s.logger.Error("All upstreams are unhealthy due to reasons other than high latency or error rate.")
	}

	// TODO(polsar): (Once the task above is complete.) If `alwaysRoute` is true, the only way we can get here is if
	//  there are no upstreams in `upstreamsByPriority`. This shouldn't be possible, so we should log a critical error.
	return "", DefaultNoHealthyUpstreamsError
}

// Partitions the given upstreams by their health status.
func partitionUpstreams(upstreamsByPriority types.PriorityToUpstreamsMap) map[conf.UnhealthyReason]types.PriorityToUpstreamsMap {
	statusToUpstreamsByPriority := make(map[conf.UnhealthyReason]types.PriorityToUpstreamsMap)

	for priority, upstreams := range upstreamsByPriority {
		for _, upstream := range upstreams {
			status := upstream.HealthStatus

			if upstreamsByPriorityForStatus, statusExists := statusToUpstreamsByPriority[status]; statusExists {
				// The priority-to-upstreams map exists for the status.
				if upstreamsForStatusAndPriority, priorityExists := upstreamsByPriorityForStatus[priority]; priorityExists {
					// The upstreams slice exists for the status and priority, so append to it.
					upstreamsByPriorityForStatus[priority] = append(upstreamsForStatusAndPriority, upstream)
				} else {
					// The upstreams slice does not exist for the status and priority, so create it.
					upstreamsByPriorityForStatus[priority] = []*conf.UpstreamConfig{upstream}
				}
			} else {
				// The priority-to-upstreams map does not exist for the status, so create it.
				statusToUpstreamsByPriority[status] = types.PriorityToUpstreamsMap{
					priority: []*conf.UpstreamConfig{upstream},
				}
			}
		}
	}

	return statusToUpstreamsByPriority
}

// Returns the first upstream with the highest priority in the given map. Note the in our case the highest priority
// corresponds to the lowest int value.
func getHighestPriorityUpstream(upstreamsByPriority types.PriorityToUpstreamsMap) *conf.UpstreamConfig {
	priorities := maps.Keys(upstreamsByPriority)

	if len(priorities) == 0 {
		return nil
	}

	maxPriority := slices.Min(priorities)
	upstreams := upstreamsByPriority[maxPriority]

	if len(upstreams) == 0 {
		// If a priority is a key in the passed map, there must be at least one upstream for it.
		panic("No upstreams found!")
	}

	return upstreams[0]
}
