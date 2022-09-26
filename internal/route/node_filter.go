package route

import (
	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
)

type RequestMetadata struct{}

type NodeFilter interface {
	Apply(requestMetadata *RequestMetadata, upstreamConfig *config.UpstreamConfig) bool
}

type AndFilter struct {
	filters []NodeFilter
}

func (a *AndFilter) Apply(requestMetadata *RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	var result = true

	for filterIndex := range a.filters {
		var filter = a.filters[filterIndex]
		if !filter.Apply(requestMetadata, upstreamConfig) {
			result = false
			break
		}
	}

	return result
}

type IsHealthy struct {
	healthCheckManager checks.HealthCheckManager
}

func (f *IsHealthy) Apply(_ *RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	var upstreamStatus = f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	return upstreamStatus.PeerCheck.IsPassing() && upstreamStatus.SyncingCheck.IsPassing()
}

type IsAtGlobalMaxHeight struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsAtGlobalMaxHeight) Apply(_ *RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	maxHeight := f.chainMetadataStore.GetGlobalMaxHeight()

	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)

	return upstreamStatus.BlockHeightCheck.IsPassing(maxHeight)
}

type IsAtMaxHeightForGroup struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsAtMaxHeightForGroup) Apply(_ *RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	maxHeightForGroup := f.chainMetadataStore.GetMaxHeightForGroup(upstreamConfig.GroupID)

	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)

	return upstreamStatus.BlockHeightCheck.IsPassing(maxHeightForGroup)
}

func CreateNodeFilter(
	filterNames []NodeFilterType,
	manager checks.HealthCheckManager,
	store *metadata.ChainMetadataStore,
) NodeFilter {
	var filters = make([]NodeFilter, len(filterNames))
	for i := range filterNames {
		filters[i] = CreateSingleNodeFilter(filterNames[i], manager, store)
	}

	return &AndFilter{filters}
}

func CreateSingleNodeFilter(
	filterName NodeFilterType,
	manager checks.HealthCheckManager,
	store *metadata.ChainMetadataStore,
) NodeFilter {
	switch filterName {
	case Healthy:
		return &IsHealthy{manager}
	case GlobalMaxHeight:
		return &IsAtGlobalMaxHeight{
			healthCheckManager: manager,
			chainMetadataStore: store,
		}
	case MaxHeightForGroup:
		return &IsAtMaxHeightForGroup{
			healthCheckManager: manager,
			chainMetadataStore: store,
		}
	default:
		panic("Unknown filter type " + filterName + "!")
	}
}

type NodeFilterType string

const (
	Healthy           NodeFilterType = "healthy"
	GlobalMaxHeight   NodeFilterType = "globalMaxHeight"
	MaxHeightForGroup NodeFilterType = "maxHeightForGroup"
)
