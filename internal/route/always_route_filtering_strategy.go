package route

import (
	"reflect"
	"strings"

	"github.com/satsuma-data/node-gateway/internal/config"
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
		upstreams := s.FilterUpstreams(upstreamsByPriority, requestMetadata, filters)

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

// FilterUpstreams filters upstreams based on the provided NodeFilter.
// WARNING: If a given priority does not have any healthy upstreams,
// it will not be included in the returned map.
func (s *AlwaysRouteFilteringStrategy) FilterUpstreams(
	upstreamsByPriority types.PriorityToUpstreamsMap,
	requestMetadata metadata.RequestMetadata,
	nodeFilters []NodeFilter,
) types.PriorityToUpstreamsMap {
	priorityToHealthyUpstreams := make(types.PriorityToUpstreamsMap)

	// Iterate over each priority and filter the associated upstreams.
	for priority, upstreamConfigs := range upstreamsByPriority {
		s.Logger.Debug(
			"Determining healthy upstreams at priority.",
			zap.Int("priority", priority),
			zap.Any("upstreams", upstreamConfigs),
		)

		filteredUpstreams := make([]*config.UpstreamConfig, 0)

		// Iterate over each upstream and apply the filters.
		for _, upstreamConfig := range upstreamConfigs {
			pass := true
			for _, nodeFilter := range nodeFilters {
				pass = nodeFilter.Apply(requestMetadata, upstreamConfig, len(upstreamConfigs))
				if !pass {
					break
				}
			}

			if pass {
				filteredUpstreams = append(filteredUpstreams, upstreamConfig)
			}
		}

		// Only add the priority to the map if there is at least one healthy upstream.
		if len(filteredUpstreams) > 0 {
			priorityToHealthyUpstreams[priority] = filteredUpstreams
		}
	}

	return priorityToHealthyUpstreams
}

// removeFilters returns a new slice of filters with the given filter type removed.
func removeFilters(filters []NodeFilter, filterToRemove NodeFilterType) []NodeFilter {
	retFilters := make([]NodeFilter, 0)

	for _, filter := range filters {
		if GetFilterTypeName(filter) != filterToRemove {
			retFilters = append(retFilters, filter)
		}
	}

	return retFilters
}

func GetFilterTypeName(v interface{}) NodeFilterType {
	t := reflect.TypeOf(v)

	// If it's a pointer, get the element type.
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Extract the name of the type and remove the package path.
	typeName := t.String()
	lastDotIndex := strings.LastIndex(typeName, ".")

	if lastDotIndex != -1 {
		// Remove the package path, keep only the type name.
		typeName = typeName[lastDotIndex+1:]
	}

	return NodeFilterType(typeName)
}
