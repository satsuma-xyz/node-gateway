package route

import (
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/satsuma-data/node-gateway/internal/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
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
		logger:             zap.L(),
		maxBlocksBehind:    10,
	}

	blockHeightCheck.EXPECT().GetError().Return(nil)

	blockHeightCheck.EXPECT().GetBlockHeight().Return(85).Once()
	assert.False(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig))

	blockHeightCheck.EXPECT().GetBlockHeight().Return(91).Once()
	assert.True(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig))

	blockHeightCheck.EXPECT().GetBlockHeight().Return(99).Once()
	assert.True(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig))
}

func TestMethodsAllowedFilter_Apply(t *testing.T) {
	fullNodeConfig := config.UpstreamConfig{NodeType: config.Full}
	fullNodeConfigWithArchiveMethodEnabled := config.UpstreamConfig{
		NodeType: config.Full,
		Methods: config.MethodsConfig{
			Enabled: map[string]bool{
				"eth_getBalance": true,
			},
		},
	}
	fullNodeConfigWithFullMethodDisabled := config.UpstreamConfig{
		NodeType: config.Full,
		Methods: config.MethodsConfig{
			Disabled: map[string]bool{
				"eth_getBlockByNumber": true,
			},
		},
	}
	archiveNodeConfig := config.UpstreamConfig{NodeType: config.Archive}
	archiveNodeConfigWithMethodDisabled := config.UpstreamConfig{
		NodeType: config.Archive,
		Methods: config.MethodsConfig{
			Disabled: map[string]bool{
				"eth_getBalance": true,
			},
		},
	}

	stateMethodMetadata := metadata.RequestMetadata{Methods: []string{"eth_getBalance"}}
	nonStateMethodMetadata := metadata.RequestMetadata{Methods: []string{"eth_getBlockByNumber"}}
	// Batch requests
	stateMethodsMetadata := metadata.RequestMetadata{Methods: []string{"eth_getBalance", "eth_getBlockNumber"}}

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
		{"stateMethodFullNodeWithArchiveMethodEnabled",
			args{stateMethodMetadata, &fullNodeConfigWithArchiveMethodEnabled}, true},
		{"stateMethodArchiveNode", args{stateMethodMetadata, &archiveNodeConfig}, true},
		{"stateMethodArchiveNodeWithMethodDisabled",
			args{stateMethodMetadata, &archiveNodeConfigWithMethodDisabled}, false},
		{"nonStateMethodFullNode", args{nonStateMethodMetadata, &fullNodeConfig}, true},
		{"nonStateMethodFullNodeWithMethodDisabled",
			args{nonStateMethodMetadata, &fullNodeConfigWithFullMethodDisabled}, false},
		{"nonStateMethodArchiveNode", args{nonStateMethodMetadata, &archiveNodeConfig}, true},
		{"batchRequestsStateMethodsFullNode", args{stateMethodsMetadata, &fullNodeConfig}, false},
		{"batchRequestsStateMethodsFullNodeWithArchiveMethodEnabled",
			args{stateMethodsMetadata, &fullNodeConfigWithArchiveMethodEnabled}, true},
		{"batchRequestsStateMethodsArchiveNode", args{stateMethodsMetadata, &archiveNodeConfig}, true},
		{"batchRequestsStateMethodsArchiveNodeWithMethodDisabled",
			args{stateMethodsMetadata, &archiveNodeConfigWithMethodDisabled}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &AreMethodsAllowed{logger: zap.L()}
			ok := f.Apply(tt.args.requestMetadata, tt.args.upstreamConfig)
			assert.Equalf(t, tt.want, ok, "Apply(%v, %v)", tt.args.requestMetadata, tt.args.upstreamConfig)
		})
	}
}
