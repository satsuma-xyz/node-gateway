package route

import "go.uber.org/zap"

type FilteringRoutingStrategy struct {
	nodeFilter      NodeFilter
	backingStrategy RoutingStrategy
}

func (s *FilteringRoutingStrategy) RouteNextRequest(upstreamsByPriority map[int][]string) (string, error) {
	var filteredUpstreams = s.filter(upstreamsByPriority)
	return s.backingStrategy.RouteNextRequest(filteredUpstreams)
}

func (s *FilteringRoutingStrategy) filter(upstreamsByPriority map[int][]string) map[int][]string {
	priorityToHealthyUpstreams := make(map[int][]string)

	for priority, upstreamIDs := range upstreamsByPriority {
		zap.L().Debug("Determining healthy upstreams at priority.", zap.Int("priority", priority), zap.Any("upstreams", upstreamIDs))

		var filteredUpstreams = make([]string, 0)
		for _, upstreamID := range upstreamIDs {
			if s.nodeFilter.Apply(nil, upstreamID) {
				filteredUpstreams = append(filteredUpstreams, upstreamID)
			}
		}
		priorityToHealthyUpstreams[priority] = filteredUpstreams
	}

	return priorityToHealthyUpstreams
}
