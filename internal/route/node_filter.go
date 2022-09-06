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

type IsHealthyAndAtGlobalMaxHeightFilter struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsHealthyAndAtGlobalMaxHeightFilter) Apply(requestMetadata *RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	var maxHeight = f.chainMetadataStore.GetGlobalMaxHeight()

	var upstreamStatus = f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)

	return upstreamStatus.IsHealthy(maxHeight)
}

type IsHealthyAndAtMaxHeightForGroupFilter struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsHealthyAndAtMaxHeightForGroupFilter) Apply(requestMetadata *RequestMetadata, upstreamConfig *config.UpstreamConfig) bool {
	var maxHeightForGroup = f.chainMetadataStore.GetMaxHeightForGroup(upstreamConfig.GroupID)

	var upstreamStatus = f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)

	return upstreamStatus.IsHealthy(maxHeightForGroup)
}
