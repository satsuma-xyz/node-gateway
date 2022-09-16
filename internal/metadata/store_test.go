package metadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChainMetadataStore_GetGlobalMaxHeight(t *testing.T) {
	var store = NewChainMetadataStore()

	store.Start()

	assert.Equal(t, uint64(0), store.GetGlobalMaxHeight())

	emitBlockHeight(store, "group1", uint64(10))
	assert.Equal(t, uint64(10), store.GetGlobalMaxHeight())

	emitBlockHeight(store, "group2", 12)
	assert.Equal(t, uint64(12), store.GetGlobalMaxHeight())

	emitBlockHeight(store, "group1", uint64(11))
	// Non-increasing block heights are ignored.
	assert.Equal(t, uint64(12), store.GetGlobalMaxHeight())
}

func TestChainMetadataStore_GetMaxHeightForGroup(t *testing.T) {
	var store = NewChainMetadataStore()

	store.Start()

	assert.Equal(t, uint64(0), store.GetMaxHeightForGroup("group1"))

	emitBlockHeight(store, "group1", uint64(10))
	assert.Equal(t, uint64(10), store.GetMaxHeightForGroup("group1"))
	assert.Equal(t, uint64(0), store.GetMaxHeightForGroup("group2"))

	emitBlockHeight(store, "group2", 12)
	// Updates for other groups are ignored.
	assert.Equal(t, uint64(10), store.GetMaxHeightForGroup("group1"))
	assert.Equal(t, uint64(12), store.GetMaxHeightForGroup("group2"))

	emitBlockHeight(store, "group1", uint64(11))
	assert.Equal(t, uint64(11), store.GetMaxHeightForGroup("group1"))

	emitBlockHeight(store, "group1", uint64(9))
	// Non-increasing block heights are ignored.
	assert.Equal(t, uint64(11), store.GetMaxHeightForGroup("group1"))
}

func emitBlockHeight(store *ChainMetadataStore, groupID string, blockHeight uint64) {
	store.ProcessUpdate(BlockHeightUpdate{
		GroupID:     groupID,
		BlockHeight: blockHeight,
	})
}
