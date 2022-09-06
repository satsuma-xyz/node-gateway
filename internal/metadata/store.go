package metadata

type BlockHeightUpdate struct {
	GroupID     string
	BlockHeight uint64
}

type ChainMetadataStore struct {
	blockHeightChannel <-chan BlockHeightUpdate
	maxHeightByGroupID map[string]uint64
	globalMaxHeight    uint64
}

func NewChainMetadataStore(blockHeightChannel <-chan BlockHeightUpdate) *ChainMetadataStore {
	return &ChainMetadataStore{
		blockHeightChannel: blockHeightChannel,
		maxHeightByGroupID: make(map[string]uint64),
	}
}

func (c *ChainMetadataStore) Start() {
	go func() {
		for {
			var update = <-c.blockHeightChannel
			c.globalMaxHeight = max(c.globalMaxHeight, update.BlockHeight)
			c.updateHeightForGroup(update.GroupID, update.BlockHeight)
		}
	}()
}

func (c *ChainMetadataStore) updateHeightForGroup(groupID string, currentBlockHeight uint64) {
	var oldHeight = c.maxHeightByGroupID[groupID]

	var newMaxHeight = max(oldHeight, currentBlockHeight)

	c.maxHeightByGroupID[groupID] = newMaxHeight
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}

	return b
}

func (c *ChainMetadataStore) GetGlobalMaxHeight() uint64 {
	return c.globalMaxHeight
}

func (c *ChainMetadataStore) GetMaxHeightForGroup(groupID string) uint64 {
	return c.maxHeightByGroupID[groupID]
}
