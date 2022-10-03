package route

import (
	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"go.uber.org/zap"
)

const DefaultMaxBlocksBehind = 10

type NodeFilter interface {
	Apply(requestMetadata metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig) bool
}

type AndFilter struct {
	filters    []NodeFilter
	isTopLevel bool
}

func (a *AndFilter) Apply(requestMetadata metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	var result = true

	for filterIndex := range a.filters {
		filter := a.filters[filterIndex]

		ok := filter.Apply(requestMetadata, upstreamConfig)
		if !ok {
			result = false
			break
		}
	}

	if result && a.isTopLevel {
		zap.L().Debug("Upstream passed all filters for request.", zap.String("upstreamID", upstreamConfig.ID), zap.Any("requestMetadata", requestMetadata))
	}

	return result
}

type HasEnoughPeers struct {
	healthCheckManager checks.HealthCheckManager
	minimumPeerCount   uint64
}

func (f *HasEnoughPeers) Apply(_ metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	peerCheck, _ := upstreamStatus.PeerCheck.(*checks.PeerCheck)

	if peerCheck.ShouldRun {
		if peerCheck.Err != nil {
			zap.L().Debug("HasEnoughPeers failed: most recent health check did not succeed.", zap.Error(peerCheck.Err))
			return false
		}

		if peerCheck.PeerCount >= f.minimumPeerCount {
			return true
		}

		zap.L().Debug("HasEnoughPeers failed.", zap.Uint64("MinimumPeerCount", f.minimumPeerCount), zap.Uint64("ActualPeerCount", peerCheck.PeerCount))

		return false
	}

	return true
}

type IsDoneSyncing struct {
	healthCheckManager checks.HealthCheckManager
}

func (f *IsDoneSyncing) Apply(_ metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)

	isSyncingCheck, _ := upstreamStatus.SyncingCheck.(*checks.SyncingCheck)

	if isSyncingCheck.ShouldRun {
		if isSyncingCheck.Err != nil {
			zap.L().Debug("IsDoneSyncing failed: most recent health check did not succeed.", zap.Error(isSyncingCheck.Err))
			return false
		}

		if !isSyncingCheck.IsSyncing {
			return true
		}

		zap.L().Debug("Upstream is still syncing!", zap.String("UpstreamID", upstreamConfig.ID))

		return false
	}

	return true
}

type IsCloseToGlobalMaxHeight struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
	maxBlocksBehind    uint64
}

func (f *IsCloseToGlobalMaxHeight) Apply(
	_ metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
) bool {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	check := upstreamStatus.BlockHeightCheck

	checkIsHealthy := check.GetError() == nil
	if !checkIsHealthy {
		zap.L().Debug("IsCloseToGlobalMaxHeight failed: most recent health check did not succeed.", zap.Error(check.GetError()))
		return false
	}

	maxHeight := f.chainMetadataStore.GetGlobalMaxHeight()
	upstreamHeight := check.GetBlockHeight()
	isClose := upstreamHeight+f.maxBlocksBehind >= maxHeight

	if isClose {
		return true
	}

	zap.L().Debug("Upstream too far behind global max height! UpstreamHeight: %d, MaxHeight: %d", zap.Uint64("UpstreamHeight", upstreamHeight), zap.Uint64("MaxHeight", maxHeight))

	return false
}

type IsAtMaxHeightForGroup struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsAtMaxHeightForGroup) Apply(_ metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	check := upstreamStatus.BlockHeightCheck

	if check.GetError() != nil {
		zap.L().Debug("IsCloseToGlobalMaxHeight failed: most recent health check did not succeed.", zap.Error(check.GetError()))
		return false
	}

	maxHeightForGroup := f.chainMetadataStore.GetMaxHeightForGroup(upstreamConfig.GroupID)
	if check.GetBlockHeight() >= maxHeightForGroup {
		return true
	}

	zap.L().Debug("Upstream not at max height for group!", zap.Uint64("UpstreamHeight", check.GetBlockHeight()), zap.Uint64("MaxHeightForGroup", maxHeightForGroup))

	return false
}

type SimpleIsStatePresent struct{}

func (f *SimpleIsStatePresent) Apply(
	requestMetadata metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
) bool {
	if requestMetadata.IsStateRequired {
		return upstreamConfig.NodeType == config.Archive
	}

	return true
}

func CreateNodeFilter(
	filterNames []NodeFilterType,
	manager checks.HealthCheckManager,
	store *metadata.ChainMetadataStore,
	routingConfig *config.RoutingConfig,
) NodeFilter {
	var filters = make([]NodeFilter, len(filterNames))
	for i := range filterNames {
		filters[i] = CreateSingleNodeFilter(filterNames[i], manager, store, routingConfig)
	}

	return &AndFilter{filters, true}
}

func CreateSingleNodeFilter(
	filterName NodeFilterType,
	manager checks.HealthCheckManager,
	store *metadata.ChainMetadataStore,
	routingConfig *config.RoutingConfig,
) NodeFilter {
	switch filterName {
	case Healthy:
		hasEnoughPeers := HasEnoughPeers{
			healthCheckManager: manager,
			minimumPeerCount:   checks.MinimumPeerCount,
		}
		isDoneSyncing := IsDoneSyncing{healthCheckManager: manager}

		return &AndFilter{filters: []NodeFilter{&hasEnoughPeers, &isDoneSyncing}}
	case GlobalMaxHeight:
		return &IsCloseToGlobalMaxHeight{
			healthCheckManager: manager,
			chainMetadataStore: store,
			maxBlocksBehind:    0,
		}
	case NearGlobalMaxHeight:
		maxBlocksBehind := DefaultMaxBlocksBehind
		if routingConfig.MaxBlocksBehind != 0 {
			maxBlocksBehind = routingConfig.MaxBlocksBehind
		}

		return &IsCloseToGlobalMaxHeight{
			healthCheckManager: manager,
			chainMetadataStore: store,
			maxBlocksBehind:    uint64(maxBlocksBehind),
		}
	case MaxHeightForGroup:
		return &IsAtMaxHeightForGroup{
			healthCheckManager: manager,
			chainMetadataStore: store,
		}
	case SimpleStatePresent:
		return &SimpleIsStatePresent{}
	default:
		panic("Unknown filter type " + filterName + "!")
	}
}

type NodeFilterType string

const (
	Healthy             NodeFilterType = "healthy"
	GlobalMaxHeight     NodeFilterType = "globalMaxHeight"
	NearGlobalMaxHeight NodeFilterType = "nearGlobalMaxHeight"
	MaxHeightForGroup   NodeFilterType = "maxHeightForGroup"
	SimpleStatePresent  NodeFilterType = "simpleStatePresent"
)
