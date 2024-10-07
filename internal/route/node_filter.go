package route

import (
	"reflect"
	"strings"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"go.uber.org/zap"
)

const DefaultMaxBlocksBehind = 10

type NodeFilter interface {
	Apply(
		requestMetadata metadata.RequestMetadata,
		upstreamConfig *config.UpstreamConfig,
		numUpstreamsInPriorityGroup int,
	) bool
}

type AndFilter struct {
	logger     *zap.Logger
	filters    []NodeFilter
	isTopLevel bool
}

func NewAndFilter(filters []NodeFilter, logger *zap.Logger) *AndFilter {
	return &AndFilter{
		logger:     logger,
		filters:    filters,
		isTopLevel: true,
	}
}

func (a *AndFilter) Apply(requestMetadata metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig, numUpstreamsInPriorityGroup int) bool {
	var result = true

	for filterIndex := range a.filters {
		filter := a.filters[filterIndex]

		ok := filter.Apply(requestMetadata, upstreamConfig, numUpstreamsInPriorityGroup)
		if !ok {
			result = false
			break
		}
	}

	if result && a.isTopLevel {
		a.logger.Debug("Upstream passed all filters for request.",
			zap.String("upstreamID", upstreamConfig.ID),
			zap.Any("RequestMetadata", requestMetadata),
		)
	}

	return result
}

type HasEnoughPeers struct {
	healthCheckManager checks.HealthCheckManager
	logger             *zap.Logger
	minimumPeerCount   uint64
}

func (f *HasEnoughPeers) Apply(_ metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig, _ int) bool {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	peerCheck, _ := upstreamStatus.PeerCheck.(*checks.PeerCheck)

	if peerCheck.ShouldRun {
		if peerCheck.Err != nil {
			f.logger.Debug("HasEnoughPeers failed: most recent health check did not succeed.", zap.String("upstreamID", upstreamConfig.ID), zap.Error(peerCheck.Err))
			return false
		}

		if peerCheck.PeerCount >= f.minimumPeerCount {
			return true
		}

		f.logger.Debug("HasEnoughPeers failed.",
			zap.String("UpstreamID", upstreamConfig.ID),
			zap.Uint64("MinimumPeerCount", f.minimumPeerCount),
			zap.Uint64("ActualPeerCount", peerCheck.PeerCount),
		)

		return false
	}

	return true
}

type IsDoneSyncing struct {
	healthCheckManager checks.HealthCheckManager
	logger             *zap.Logger
}

func (f *IsDoneSyncing) Apply(_ metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig, _ int) bool {
	upstreamStatus := f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)

	isSyncingCheck, _ := upstreamStatus.SyncingCheck.(*checks.SyncingCheck)

	if isSyncingCheck.ShouldRun {
		if isSyncingCheck.Err != nil {
			f.logger.Debug(
				"IsDoneSyncing failed: most recent health check did not succeed.",
				zap.Error(isSyncingCheck.Err),
				zap.String("UpstreamID", upstreamConfig.ID),
			)

			return false
		}

		if !isSyncingCheck.IsSyncing {
			return true
		}

		f.logger.Debug("Upstream is still syncing!", zap.String("UpstreamID", upstreamConfig.ID))

		return false
	}

	return true
}

type IsErrorRateAcceptable struct {
	HealthCheckManager checks.HealthCheckManager
}

func (f *IsErrorRateAcceptable) Apply(requestMetadata metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig, _ int) bool {
	upstreamStatus := f.HealthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	return upstreamStatus.ErrorCheck.IsPassing(requestMetadata.Methods)
}

type IsLatencyAcceptable struct {
	HealthCheckManager checks.HealthCheckManager
}

func (f *IsLatencyAcceptable) Apply(requestMetadata metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig, _ int) bool {
	upstreamStatus := f.HealthCheckManager.GetUpstreamStatus(upstreamConfig.ID)
	return upstreamStatus.LatencyCheck.IsPassing(requestMetadata.Methods)
}

type IsCloseToGlobalMaxHeight struct {
	chainMetadataStore *metadata.ChainMetadataStore
	logger             *zap.Logger
	maxBlocksBehind    uint64
}

func (f *IsCloseToGlobalMaxHeight) Apply(
	_ metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
	_ int,
) bool {
	status := f.chainMetadataStore.GetBlockHeightStatus(upstreamConfig.GroupID, upstreamConfig.ID)

	if status.Error != nil {
		f.logger.Debug("IsCloseToGlobalMaxHeight failed: most recent health check did not succeed.",
			zap.Error(status.Error),
			zap.String("UpstreamID", upstreamConfig.ID),
		)

		return false
	}

	isClose := status.BlockHeight+f.maxBlocksBehind >= status.GlobalMaxBlockHeight
	if isClose {
		return true
	}

	f.logger.Debug(
		"Upstream too far behind global max height!",
		zap.String("UpstreamID", upstreamConfig.ID),
		zap.Uint64("UpstreamHeight", status.BlockHeight),
		zap.Uint64("MaxHeight", status.GlobalMaxBlockHeight),
	)

	return false
}

type IsAtMaxHeightForGroup struct {
	chainMetadataStore *metadata.ChainMetadataStore
	logger             *zap.Logger
}

func (f *IsAtMaxHeightForGroup) Apply(_ metadata.RequestMetadata, upstreamConfig *config.UpstreamConfig, numUpstreamsInPriorityGroup int) bool {
	status := f.chainMetadataStore.GetBlockHeightStatus(upstreamConfig.GroupID, upstreamConfig.ID)

	if status.Error != nil {
		f.logger.Debug("IsAtMaxHeightForGroup failed: most recent health check did not succeed.",
			zap.String("UpstreamID", upstreamConfig.ID),
			zap.Error(status.Error),
		)

		return false
	}

	// This allows us to successfully route requests if an upstream travels back in block
	// height, *only in the case where there's only 1 upstream in the group.*
	// This is a workaround for the fact that we set the max height in a group based on
	// the max height across all rounds of checks instead of within 1 round. We should fix
	// this so we can route properly to upstreams that travel back in height if there are
	// multiple upstreams in the group.
	if numUpstreamsInPriorityGroup == 1 {
		f.logger.Debug("IsAtMaxHeightForGroup passing because there's only 1 upstream in the group.")
		return true
	}

	if status.BlockHeight >= status.GroupMaxBlockHeight {
		return true
	}

	f.logger.Debug(
		"Upstream not at max height for group!",
		zap.String("UpstreamID", upstreamConfig.ID),
		zap.Uint64("UpstreamHeight", status.BlockHeight),
		zap.Uint64("MaxHeightForGroup", status.GroupMaxBlockHeight),
	)

	return false
}

func isArchiveNodeMethod(method string) bool {
	switch method {
	case "eth_getBalance", "eth_getStorageAt", "eth_getTransactionCount", "eth_getCode", "eth_call", "eth_estimateGas":
		// List of state methods: https://ethereum.org/en/developers/docs/apis/json-rpc/#state_methods
		return true
	case "trace_filter", "trace_block", "trace_get", "trace_transaction", "trace_call", "trace_callMany",
		"trace_rawTransaction", "trace_replayBlockTransactions", "trace_replayTransaction":
		return true
	default:
		return false
	}
}

type AreMethodsAllowed struct {
	logger *zap.Logger
}

func (f *AreMethodsAllowed) Apply(
	requestMetadata metadata.RequestMetadata,
	upstreamConfig *config.UpstreamConfig,
	_ int,
) bool {
	for _, method := range requestMetadata.Methods {
		// Check methods that are have been disabled on the upstream.
		if ok := upstreamConfig.Methods.Disabled[method]; ok {
			f.logger.Debug(
				"Upstream method is disabled! Skipping upstream.",
				zap.String("UpstreamID", upstreamConfig.ID),
				zap.Any("RequestMetadata", requestMetadata),
			)

			return false
		}

		if isArchiveNodeMethod(method) && upstreamConfig.NodeType == config.Full {
			// Check if method has been explicitly enabled on the upstream.
			if ok := upstreamConfig.Methods.Enabled[method]; !ok {
				f.logger.Debug(
					"Upstream method is archive, nodeType is not archive, and method has not been enabled! Skipping upstream.",
					zap.String("UpstreamID", upstreamConfig.ID),
					zap.Any("RequestMetadata", requestMetadata),
				)

				return false
			}
		}
	}

	return true
}

func CreateNodeFilter(
	filterNames []NodeFilterType,
	manager checks.HealthCheckManager,
	store *metadata.ChainMetadataStore,
	logger *zap.Logger,
	routingConfig *config.RoutingConfig,
) NodeFilter {
	var filters = make([]NodeFilter, len(filterNames))
	for i := range filterNames {
		filters[i] = CreateSingleNodeFilter(filterNames[i], manager, store, logger, routingConfig)
	}

	return &AndFilter{logger: logger, filters: filters, isTopLevel: true}
}

func CreateSingleNodeFilter(
	filterName NodeFilterType,
	manager checks.HealthCheckManager,
	store *metadata.ChainMetadataStore,
	logger *zap.Logger,
	routingConfig *config.RoutingConfig,
) NodeFilter {
	switch filterName {
	case Healthy:
		hasEnoughPeers := HasEnoughPeers{
			healthCheckManager: manager,
			logger:             logger,
			minimumPeerCount:   checks.MinimumPeerCount,
		}

		return &AndFilter{
			filters: []NodeFilter{
				&hasEnoughPeers,
			},
			logger: logger,
		}
	case NearGlobalMaxHeight:
		maxBlocksBehind := DefaultMaxBlocksBehind
		if routingConfig.MaxBlocksBehind != 0 {
			maxBlocksBehind = routingConfig.MaxBlocksBehind
		}

		return &IsCloseToGlobalMaxHeight{
			chainMetadataStore: store,
			logger:             logger,
			maxBlocksBehind:    uint64(maxBlocksBehind),
		}
	case MaxHeightForGroup:
		return &IsAtMaxHeightForGroup{
			chainMetadataStore: store,
			logger:             logger,
		}
	case MethodsAllowed:
		return &AreMethodsAllowed{logger: logger}
	case ErrorRateAcceptable:
		panic("ErrorRateAcceptable filter is not implemented!")
	case LatencyAcceptable:
		panic("LatencyAcceptable filter is not implemented!")
	default:
		panic("Unknown filter type " + filterName + "!")
	}
}

type NodeFilterType string

const (
	Healthy             NodeFilterType = "healthy"
	NearGlobalMaxHeight NodeFilterType = "nearGlobalMaxHeight"
	MaxHeightForGroup   NodeFilterType = "maxHeightForGroup"
	MethodsAllowed      NodeFilterType = "methodsAllowed"
	ErrorRateAcceptable NodeFilterType = "errorRateAcceptable"
	LatencyAcceptable   NodeFilterType = "latencyAcceptable"
)

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
