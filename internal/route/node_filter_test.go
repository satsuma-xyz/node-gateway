package route

import (
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type AlwaysPass struct{}

func (AlwaysPass) Apply(metadata.RequestMetadata, *config.UpstreamConfig, int) bool {
	return true
}

type AlwaysFail struct{}

func (AlwaysFail) Apply(metadata.RequestMetadata, *config.UpstreamConfig, int) bool {
	return false
}

const (
	GroupID1    = "group1"
	GroupID2    = "group2"
	UpstreamID1 = "upstream1"
	UpstreamID2 = "upstream2"
)

func TestAndFilter_Apply(t *testing.T) {
	type fields struct {
		filters []NodeFilter
	}

	type argsType struct {
		upstreamConfig  *config.UpstreamConfig
		requestMetadata metadata.RequestMetadata
	}

	args := argsType{upstreamConfig: cfg("test-node")}

	tests := []struct {
		args   argsType
		name   string
		fields fields
		want   bool
	}{
		{
			args,
			"No filters",
			fields{},
			true,
		},
		{
			args,
			"One passing filter",
			fields{[]NodeFilter{AlwaysPass{}}},
			true,
		},
		{
			args,
			"Multiple passing filters",
			fields{[]NodeFilter{AlwaysPass{}, AlwaysPass{}, AlwaysPass{}}},
			true,
		},
		{
			args,
			"One failing filter",
			fields{[]NodeFilter{AlwaysFail{}}},
			false,
		},
		{
			args,
			"Multiple passing and one failing",
			fields{[]NodeFilter{AlwaysPass{}, AlwaysPass{}, AlwaysFail{}}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AndFilter{
				filters: tt.fields.filters,
			}
			ok := a.Apply(tt.args.requestMetadata, tt.args.upstreamConfig, 1)
			assert.Equalf(t, tt.want, ok, "Apply(%v, %v)", tt.args.requestMetadata, tt.args.upstreamConfig)
		})
	}
}

func TestIsCloseToGlobalMaxHeight_Apply(t *testing.T) {
	upstreamConfig := &config.UpstreamConfig{GroupID: GroupID1, ID: UpstreamID1}

	chainMetadataStore := metadata.NewChainMetadataStore()
	chainMetadataStore.Start()

	filter := IsCloseToGlobalMaxHeight{
		chainMetadataStore: chainMetadataStore,
		logger:             zap.L(),
		maxBlocksBehind:    10,
	}

	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 100)
	emitBlockHeight(chainMetadataStore, GroupID2, UpstreamID2, 75)

	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 85)
	assert.False(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig, 1))

	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 90)
	assert.True(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig, 1))

	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 99)
	assert.True(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig, 1))

	emitError(chainMetadataStore, GroupID1, UpstreamID1, assert.AnError)
	assert.False(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig, 1))
}

func TestIsAtMaxHeightForGroup_Apply(t *testing.T) {
	upstream1Config := &config.UpstreamConfig{GroupID: GroupID1, ID: UpstreamID1}

	chainMetadataStore := metadata.NewChainMetadataStore()
	chainMetadataStore.Start()

	filter := IsAtMaxHeightForGroup{
		chainMetadataStore: chainMetadataStore,
		logger:             zap.L(),
	}

	// Set global max height to 100
	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID2, 100)

	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 85)
	assert.False(t, filter.Apply(metadata.RequestMetadata{}, upstream1Config, 2))

	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 101)
	assert.True(t, filter.Apply(metadata.RequestMetadata{}, upstream1Config, 2))

	emitError(chainMetadataStore, GroupID1, UpstreamID1, assert.AnError)
	assert.False(t, filter.Apply(metadata.RequestMetadata{}, upstream1Config, 2))
}

func TestIsAtMaxHeightForGroupOnlyUpstream_Apply(t *testing.T) {
	upstreamConfig := &config.UpstreamConfig{GroupID: GroupID1, ID: UpstreamID1}

	chainMetadataStore := metadata.NewChainMetadataStore()
	chainMetadataStore.Start()

	filter := IsAtMaxHeightForGroup{
		chainMetadataStore: chainMetadataStore,
		logger:             zap.L(),
	}

	// Set global max height to 100 for an upstream in another group.
	emitBlockHeight(chainMetadataStore, GroupID2, UpstreamID2, 100)

	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 85)
	assert.True(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig, 1))

	// Travel forward and then back in block height.
	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 101)
	emitBlockHeight(chainMetadataStore, GroupID1, UpstreamID1, 84)
	assert.True(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig, 1))

	emitError(chainMetadataStore, GroupID1, UpstreamID1, assert.AnError)
	assert.False(t, filter.Apply(metadata.RequestMetadata{}, upstreamConfig, 1))
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

	type args struct {
		upstreamConfig  *config.UpstreamConfig
		requestMetadata metadata.RequestMetadata
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			"stateMethodFullNode",
			args{
				&fullNodeConfig,
				stateMethodMetadata,
			},
			false,
		},
		{
			"stateMethodFullNodeWithArchiveMethodEnabled",
			args{
				&fullNodeConfigWithArchiveMethodEnabled,
				stateMethodMetadata,
			},
			true,
		},
		{
			"stateMethodArchiveNode",
			args{
				&archiveNodeConfig,
				stateMethodMetadata,
			},
			true,
		},
		{
			"stateMethodArchiveNodeWithMethodDisabled",
			args{
				&archiveNodeConfigWithMethodDisabled,
				stateMethodMetadata,
			},
			false,
		},
		{
			"nonStateMethodFullNode",
			args{
				&fullNodeConfig,
				nonStateMethodMetadata,
			},
			true,
		},
		{
			"nonStateMethodFullNodeWithMethodDisabled",
			args{
				&fullNodeConfigWithFullMethodDisabled,
				nonStateMethodMetadata,
			},
			false,
		},
		{
			"nonStateMethodArchiveNode",
			args{
				&archiveNodeConfig,
				nonStateMethodMetadata,
			},
			true,
		},
		{
			"batchRequestsStateMethodsFullNode",
			args{
				&fullNodeConfig,
				stateMethodsMetadata,
			},
			false,
		},
		{
			"batchRequestsStateMethodsFullNodeWithArchiveMethodEnabled",
			args{
				&fullNodeConfigWithArchiveMethodEnabled,
				stateMethodsMetadata,
			},
			true,
		},
		{
			"batchRequestsStateMethodsArchiveNode",
			args{
				&archiveNodeConfig,
				stateMethodsMetadata,
			},
			true,
		},
		{
			"batchRequestsStateMethodsArchiveNodeWithMethodDisabled",
			args{
				&archiveNodeConfigWithMethodDisabled,
				stateMethodsMetadata,
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &AreMethodsAllowed{logger: zap.L()}
			ok := f.Apply(tt.args.requestMetadata, tt.args.upstreamConfig, 1)
			assert.Equalf(t, tt.want, ok, "Apply(%v, %v)", tt.args.requestMetadata, tt.args.upstreamConfig)
		})
	}
}

func emitBlockHeight(store *metadata.ChainMetadataStore, groupID, upstreamID string, blockHeight uint64) {
	store.ProcessBlockHeightUpdate(groupID, upstreamID, blockHeight)
}

func emitError(store *metadata.ChainMetadataStore, groupID, upstreamID string, err error) {
	store.ProcessErrorUpdate(groupID, upstreamID, err)
}

func Test_getFilterTypeName(t *testing.T) {
	Assert := assert.New(t)

	Assert.Equal(
		NodeFilterType("AlwaysPass"),
		getFilterTypeName(AlwaysPass{}),
	)
	Assert.Equal(
		NodeFilterType("AlwaysFail"),
		getFilterTypeName(AlwaysFail{}),
	)
	Assert.Equal(
		NodeFilterType("AndFilter"),
		getFilterTypeName(&AndFilter{}),
	)
}
