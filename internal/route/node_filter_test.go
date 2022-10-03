package route

import (
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/satsuma-data/node-gateway/internal/types"
	"github.com/stretchr/testify/assert"
)

type AlwaysPass struct{}

func (AlwaysPass) Apply(metadata.RequestMetadata, *config.UpstreamConfig) bool {
	return true
}

type AlwaysFail struct{}

func (AlwaysFail) Apply(metadata.RequestMetadata, *config.UpstreamConfig) bool {
	return false
}

func TestAndFilter_Apply(t *testing.T) {
	type fields struct {
		filters []NodeFilter
	}

	type argsType struct { //nolint:govet // field alignment doesn't matter in tests
		requestMetadata metadata.RequestMetadata
		upstreamConfig  *config.UpstreamConfig
	}

	args := argsType{upstreamConfig: cfg("test-node")}

	tests := []struct { //nolint:govet // field alignment doesn't matter in tests
		name   string
		fields fields
		args   argsType
		want   bool
	}{
		{"No filters", fields{}, args, true},
		{"One passing filter", fields{[]NodeFilter{AlwaysPass{}}}, args, true},
		{"Multiple passing filters", fields{[]NodeFilter{AlwaysPass{}, AlwaysPass{}, AlwaysPass{}}}, args, true},
		{"One failing filter", fields{[]NodeFilter{AlwaysFail{}}}, args, false},
		{"Multiple passing and one failing", fields{[]NodeFilter{AlwaysPass{}, AlwaysPass{}, AlwaysFail{}}}, args, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AndFilter{
				filters: tt.fields.filters,
			}
			ok := a.Apply(tt.args.requestMetadata, tt.args.upstreamConfig)
			assert.Equalf(t, tt.want, ok, "Apply(%v, %v)", tt.args.requestMetadata, tt.args.upstreamConfig)
		})
	}
}

func TestIsCloseToGlobalMaxHeight_Apply(t *testing.T) {
	upstreamConfig := &config.UpstreamConfig{ID: "upstream1"}

	healthCheckManager := mocks.NewHealthCheckManager(t)

	blockHeightCheck := mocks.NewBlockHeightChecker(t)
	upstreamStatus := types.UpstreamStatus{BlockHeightCheck: blockHeightCheck}
	healthCheckManager.EXPECT().GetUpstreamStatus(upstreamConfig.ID).Return(&upstreamStatus)

	chainMetadataStore := metadata.NewChainMetadataStore()
	chainMetadataStore.Start()
	chainMetadataStore.ProcessUpdate(metadata.BlockHeightUpdate{
		GroupID:     "group1",
		BlockHeight: 100,
	})

	filter := IsCloseToGlobalMaxHeight{
		healthCheckManager: healthCheckManager,
		chainMetadataStore: chainMetadataStore,
		maxBlocksBehind:    10,
	}

	blockHeightCheck.EXPECT().GetError().Return(nil)

	blockHeightCheck.EXPECT().GetBlockHeight().Return(85).Once()

	ok := filter.Apply(metadata.RequestMetadata{}, upstreamConfig)

	assert.False(t, ok)

	blockHeightCheck.EXPECT().GetBlockHeight().Return(91).Once()

	ok = filter.Apply(metadata.RequestMetadata{}, upstreamConfig)

	assert.True(t, ok)

	blockHeightCheck.EXPECT().GetBlockHeight().Return(99).Once()

	ok = filter.Apply(metadata.RequestMetadata{}, upstreamConfig)

	assert.True(t, ok)
}

func TestSimpleIsStatePresentFilter_Apply(t *testing.T) {
	fullNodeConfig := config.UpstreamConfig{NodeType: config.Full}
	archiveNodeConfig := config.UpstreamConfig{NodeType: config.Archive}

	stateMethodMetadata := metadata.RequestMetadata{IsStateRequired: true}
	nonStateMethodMetadata := metadata.RequestMetadata{IsStateRequired: false}

	type args struct { //nolint:govet // field alignment doesn't matter in tests
		requestMetadata metadata.RequestMetadata
		upstreamConfig  *config.UpstreamConfig
	}

	tests := []struct { //nolint:govet // field alignment doesn't matter in tests
		name string
		args args
		want bool
	}{
		{"stateMethodFullNode", args{stateMethodMetadata, &fullNodeConfig}, false},
		{"stateMethodArchiveNode", args{stateMethodMetadata, &archiveNodeConfig}, true},
		{"nonStateMethodFullNode", args{nonStateMethodMetadata, &fullNodeConfig}, true},
		{"nonStateMethodArchiveNode", args{nonStateMethodMetadata, &archiveNodeConfig}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &SimpleIsStatePresent{}
			ok := f.Apply(tt.args.requestMetadata, tt.args.upstreamConfig)
			assert.Equalf(t, tt.want, ok, "Apply(%v, %v)", tt.args.requestMetadata, tt.args.upstreamConfig)
		})
	}
}
