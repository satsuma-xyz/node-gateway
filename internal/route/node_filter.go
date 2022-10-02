package route

import (
	"errors"
	"fmt"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"go.uber.org/zap"
)

const DefaultMaxBlocksBehind = 10

type NodeFilter interface {
	Apply(requestMetadata metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig) (bool, error)
}

type AndFilter struct {
	filters []NodeFilter
}

func (a *AndFilter) Apply(
	requestMetadata metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
) (bool, error) {
	var result = true
	var err error = nil

	for filterIndex := range a.filters {
		filter := a.filters[filterIndex]
		ok, err := filter.Apply(requestMetadata, upstreamConfig)
		if !ok {
			result = false
			zap.L().Info("Upstream does not pass filters for request.", zap.String("upstreamID", upstreamConfig.ID), zap.Any("requestMetadata", requestMetadata), zap.Error(err))
			break
		}
	}

	if result {
		zap.L().Debug("Upstream passed all filters for request.", zap.String("upstreamID", upstreamConfig.ID), zap.Any("requestMetadata", requestMetadata))
	}
	return result, err
}

type HasEnoughPeers struct {
	healthCheckManager checks.HealthCheckManager
	minimumPeerCount   uint64
}

func (f *HasEnoughPeers) Apply(
	_ metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
) (bool, error) {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	peerCheck := upstreamStatus.PeerCheck.(*checks.PeerCheck)
	if peerCheck.ShouldRun {
		if peerCheck.Err != nil {
			return false, fmt.Errorf("HasEnoughPeers failed: most recent health check did not succeed: %w.", peerCheck.Err)
		}

		if peerCheck.PeerCount >= f.minimumPeerCount {
			return true, nil
		}

		return false, errors.New(fmt.Sprintf("HasEnoughPeers failed: wanted %d peers but have %d.", f.minimumPeerCount, peerCheck.PeerCount))
	}

	return true, nil
}

type IsDoneSyncing struct {
	healthCheckManager checks.HealthCheckManager
}

func (f *IsDoneSyncing) Apply(
	_ metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
) (bool, error) {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)

	isSyncingCheck := upstreamStatus.SyncingCheck.(*checks.SyncingCheck)

	if isSyncingCheck.ShouldRun {
		if isSyncingCheck.Err != nil {
			return false, fmt.Errorf("IsDoneSyncing failed: most recent health check did not succeed: %w.", isSyncingCheck.Err)
		}

		if isSyncingCheck.IsSyncing == false {
			return true, nil
		}

		return false, fmt.Errorf("Upstream is still syncing!")
	}

	return true, nil
}

type IsCloseToGlobalMaxHeight struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
	maxBlocksBehind    uint64
}

func (f *IsCloseToGlobalMaxHeight) Apply(
	_ metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
) (bool, error) {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	check := upstreamStatus.BlockHeightCheck

	checkIsHealthy := check.GetError() == nil
	if !checkIsHealthy {
		return false, fmt.Errorf("IsCloseToGlobalMaxHeight failed: most recent health check did not succeed: %w.", check.GetError())
	}

	maxHeight := f.chainMetadataStore.GetGlobalMaxHeight()
	upstreamHeight := check.GetBlockHeight()
	isClose := upstreamHeight+f.maxBlocksBehind >= maxHeight

	if isClose {
		return true, nil
	}

	return false, fmt.Errorf("Upstream too far behind global max height! UpstreamHeight: %d, MaxHeight: %d", upstreamHeight, maxHeight)
}

type IsAtMaxHeightForGroup struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsAtMaxHeightForGroup) Apply(
	_ metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
) (bool, error) {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	check := upstreamStatus.BlockHeightCheck

	if check.GetError() != nil {
		return false, fmt.Errorf("IsCloseToGlobalMaxHeight failed: most recent health check did not succeed: %w.", check.GetError())
	}

	maxHeightForGroup := f.chainMetadataStore.GetMaxHeightForGroup(upstreamConfig.GroupID)
	if check.GetBlockHeight() >= maxHeightForGroup {
		return true, nil
	}

	return false, fmt.Errorf("Upstream not at max height for group! UpstreamHeight: %d, MaxHeightForGroup: %d", check.GetBlockHeight(), maxHeightForGroup)
}

type SimpleIsStatePresent struct{}

func (f *SimpleIsStatePresent) Apply(
	requestMetadata metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
) (bool, error) {
	if requestMetadata.IsStateRequired {
		return upstreamConfig.NodeType == config.Archive, nil
	}

	return true, nil
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

	return &AndFilter{filters}
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
