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
	Logger          *zap.Logger
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
	return filterUpstreams(
		upstreamsByPriority,
		requestMetadata,
		[]NodeFilter{s.NodeFilter},
		s.Logger,
	)
}

func filterUpstreams(
	upstreamsByPriority types.PriorityToUpstreamsMap,
	requestMetadata metadata.RequestMetadata,
	nodeFilters []NodeFilter,
	logger *zap.Logger,
) types.PriorityToUpstreamsMap {
	priorityToHealthyUpstreams := make(types.PriorityToUpstreamsMap)
	nodeFilter := AndFilter{
		filters: nodeFilters,
		logger:  logger,
	}

	for priority, upstreamConfigs := range upstreamsByPriority {
		logger.Debug("Determining healthy upstreams at priority.", zap.Int("priority", priority), zap.Any("upstreams", upstreamConfigs))

		filteredUpstreams := make([]*config.UpstreamConfig, 0)

		for _, upstreamConfig := range upstreamConfigs {
			ok := nodeFilter.Apply(requestMetadata, upstreamConfig, len(upstreamConfigs))
			if ok {
				filteredUpstreams = append(filteredUpstreams, upstreamConfig)
			}
		}

		priorityToHealthyUpstreams[priority] = filteredUpstreams
	}

	return priorityToHealthyUpstreams
}
