package route

import (
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

type FilteringRoutingStrategy struct {
	NodeFilter      NodeFilter
	BackingStrategy RoutingStrategy
}

func (s *FilteringRoutingStrategy) RouteNextRequest(
	upstreamsByPriority types.PriorityToUpstreamsMap,
	requestMetadata metadata.RequestMetadata,
) (string, error) {
	filteredUpstreams := s.filter(upstreamsByPriority, requestMetadata)
	return s.BackingStrategy.RouteNextRequest(filteredUpstreams, requestMetadata)
}

func (s *FilteringRoutingStrategy) filter(
	upstreamsByPriority types.PriorityToUpstreamsMap,
	requestMetadata metadata.RequestMetadata,
) types.PriorityToUpstreamsMap {
	priorityToHealthyUpstreams := make(types.PriorityToUpstreamsMap)

	for priority, upstreamConfigs := range upstreamsByPriority {
		zap.L().Debug("Determining healthy upstreams at priority.", zap.Int("priority", priority), zap.Any("upstreams", upstreamConfigs))

		filteredUpstreams := make([]*config.UpstreamConfig, 0)

		for _, upstreamConfig := range upstreamConfigs {
			ok, _ := s.NodeFilter.Apply(requestMetadata, upstreamConfig)
			if ok {
				filteredUpstreams = append(filteredUpstreams, upstreamConfig)
			}
		}

		priorityToHealthyUpstreams[priority] = filteredUpstreams
	}

	return priorityToHealthyUpstreams
}
