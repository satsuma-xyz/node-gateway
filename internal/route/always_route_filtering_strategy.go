package route

import (
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/types"
	"go.uber.org/zap"
)

type AlwaysRouteFilteringStrategy struct {
	BackingStrategy  RoutingStrategy
	Logger           *zap.Logger
	NodeFilters      []NodeFilter
	RemovableFilters []NodeFilterType
}

func (s *AlwaysRouteFilteringStrategy) RouteNextRequest(
	upstreamsByPriority types.PriorityToUpstreamsMap,
	requestMetadata metadata.RequestMetadata,
) (string, error) {
	// Create a copy of removable filters to avoid modifying the original slice.
	removableFilters := make([]NodeFilterType, len(s.RemovableFilters))
	copy(removableFilters, s.RemovableFilters)

	filters := s.NodeFilters // WARNING: Slice assignment, not a copy!
	for len(filters) > 0 {
		// Get all healthy upstreams for the current set of filters.
		upstreams := filterUpstreams(
			upstreamsByPriority,
			requestMetadata,
			filters,
			s.Logger,
		)

		// If there is at least one healthy upstream, route using the backing strategy.
		if len(upstreams) > 0 {
			return s.BackingStrategy.RouteNextRequest(upstreams, requestMetadata)
		}

		// There are no more filters to remove and no healthy upstreams found, so give up.
		if len(removableFilters) == 0 {
			break
		}

		// Remove the next filter and try again.
		idx := len(removableFilters) - 1
		nextFilterToRemove := removableFilters[idx]
		removableFilters = removableFilters[:idx]
		filters = removeFilters(filters, nextFilterToRemove)
	}

	// If all removable filters are exhausted and no healthy upstreams are found,
	// pass in the original list of upstreams to the backing strategy.
	//
	// TODO(polsar): Eventually, we want all filters to be removable. Once that is the case,
	// we should not be able to get here.
	return s.BackingStrategy.RouteNextRequest(upstreamsByPriority, requestMetadata)
}

// removeFilters returns a new slice of filters with the given filter type removed.
func removeFilters(filters []NodeFilter, filterToRemove NodeFilterType) []NodeFilter {
	retFilters := make([]NodeFilter, 0)

	for _, filter := range filters {
		if getFilterTypeName(filter) != filterToRemove {
			retFilters = append(retFilters, filter)
		}
	}

	return retFilters
}
