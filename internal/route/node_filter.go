package route

import (
	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/metadata"
)

type RequestMetadata struct{}

type NodeFilter interface {
	Apply(requestMetadata *RequestMetadata, upstreamID string) bool
}

type IsHealthyAndAtMaxHeightFilter struct {
	healthCheckManager checks.HealthCheckManager
	chainMetadataStore *metadata.ChainMetadataStore
}

func (f *IsHealthyAndAtMaxHeightFilter) Apply(_ *RequestMetadata, upstreamID string) bool {
	var maxHeight = f.chainMetadataStore.GetMaxHeight()

	var upstreamStatus = f.healthCheckManager.GetUpstreamStatus(upstreamID)

	return upstreamStatus.IsHealthy(maxHeight)
}
