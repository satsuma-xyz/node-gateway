package route

import (
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

type FilteringRoutingStrategy struct {
	nodeFilter      NodeFilter
	backingStrategy RoutingStrategy
}

func (s *FilteringRoutingStrategy) RouteNextRequest(upstreamsByPriority types.PriorityToUpstreamsMap) (string, error) {
	filteredUpstreams := s.filter(upstreamsByPriority)
	return s.backingStrategy.RouteNextRequest(filteredUpstreams)
}

func (s *FilteringRoutingStrategy) filter(upstreamsByPriority types.PriorityToUpstreamsMap) types.PriorityToUpstreamsMap {
	priorityToHealthyUpstreams := make(types.PriorityToUpstreamsMap)

	for priority, upstreamConfigs := range upstreamsByPriority {
		zap.L().Debug("Determining healthy upstreams at priority.", zap.Int("priority", priority), zap.Any("upstreams", upstreamConfigs))

		filteredUpstreams := make([]*config.UpstreamConfig, 0)

		for _, upstreamConfig := range upstreamConfigs {
			if s.nodeFilter.Apply(nil, upstreamConfig) {
				filteredUpstreams = append(filteredUpstreams, upstreamConfig)
			}
		}

		priorityToHealthyUpstreams[priority] = filteredUpstreams
	}

	return priorityToHealthyUpstreams
}
