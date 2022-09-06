package route

import (
	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
)

type RequestMetadata struct{}

type NodeFilter interface {
	Apply(requestMetadata *RequestMetadata, upstreamConfig config.UpstreamConfig) bool
}

type IsHealthyAndAtMaxHeightFilter struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsHealthyAndAtMaxHeightFilter) Apply(requestMetadata *RequestMetadata, upstreamConfig config.UpstreamConfig) bool {
	var maxHeight = f.chainMetadataStore.GetGlobalMaxHeight()

	var upstreamStatus = f.healthCheckManager.GetUpstreamStatus(upstreamConfig.ID)

	return upstreamStatus.IsHealthy(maxHeight)
}
