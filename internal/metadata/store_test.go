package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChainMetadataStore_GetBlockHeightStatus(t *testing.T) {
	store := NewChainMetadataStore()

	store.Start()

	emitBlockHeight(store, "group1", "upstream1", 101)
	status := store.GetBlockHeightStatus("group1", "upstream1")
	assert.Equal(t, uint64(101), status.BlockHeight)
	assert.Equal(t, uint64(101), status.GroupMaxBlockHeight)
	assert.Equal(t, uint64(101), status.GlobalMaxBlockHeight)

	emitBlockHeight(store, "group1", "upstream2", 102)
	status = store.GetBlockHeightStatus("group1", "upstream1")
	assert.Equal(t, uint64(101), status.BlockHeight)
	assert.Equal(t, uint64(102), status.GroupMaxBlockHeight)
	assert.Equal(t, uint64(102), status.GlobalMaxBlockHeight)

	emitBlockHeight(store, "group2", "upstream1", 201)
	status = store.GetBlockHeightStatus("group1", "upstream1")
	assert.Equal(t, uint64(101), status.BlockHeight)
	assert.Equal(t, uint64(102), status.GroupMaxBlockHeight)
	assert.Equal(t, uint64(201), status.GlobalMaxBlockHeight)

	emitError(store, "group1", "upstream1", assert.AnError)
	status = store.GetBlockHeightStatus("group1", "upstream1")
	assert.Equal(t, uint64(101), status.BlockHeight)
	assert.Equal(t, uint64(102), status.GroupMaxBlockHeight)
	assert.Equal(t, uint64(201), status.GlobalMaxBlockHeight)
	assert.Equal(t, assert.AnError, status.Error)

	emitBlockHeight(store, "group1", "upstream1", 1000)
	status = store.GetBlockHeightStatus("group1", "upstream1")
	assert.Equal(t, uint64(1000), status.BlockHeight)
	assert.Equal(t, uint64(1000), status.GroupMaxBlockHeight)
	assert.Equal(t, uint64(1000), status.GlobalMaxBlockHeight)
	assert.Nil(t, status.Error)
}

func emitBlockHeight(store *ChainMetadataStore, groupID, upstreamID string, blockHeight uint64) {
	store.ProcessBlockHeightUpdate(groupID, upstreamID, blockHeight)
}

func emitError(store *ChainMetadataStore, groupID, upstreamID string, err error) {
	store.ProcessErrorUpdate(groupID, upstreamID, err)
}
