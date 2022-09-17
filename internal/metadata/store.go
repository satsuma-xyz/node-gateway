package metadata

type BlockHeightUpdate struct {
	GroupID     string
	BlockHeight uint64
}

type ChainMetadataStore struct {
	opChannel          chan func()
	maxHeightByGroupID map[string]uint64
	globalMaxHeight    uint64
}

func NewChainMetadataStore() *ChainMetadataStore {
	return &ChainMetadataStore{
		maxHeightByGroupID: make(map[string]uint64),
		opChannel:          make(chan func()),
	}
}

func (c *ChainMetadataStore) Start() {
	go func(c *ChainMetadataStore) {
		for op := range c.opChannel {
			op()
		}
	}(c)
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
	var returnChannel = make(chan uint64)
	c.opChannel <- func() {
		returnChannel <- c.globalMaxHeight
		close(returnChannel)
	}

	var result = <-returnChannel

	return result
}

func (c *ChainMetadataStore) GetMaxHeightForGroup(groupID string) uint64 {
	var returnChannel = make(chan uint64)
	c.opChannel <- func() {
		returnChannel <- c.maxHeightByGroupID[groupID]
	}

	return <-returnChannel
}

func (c *ChainMetadataStore) ProcessUpdate(update BlockHeightUpdate) {
	c.opChannel <- func() {
		c.globalMaxHeight = max(c.globalMaxHeight, update.BlockHeight)
		c.updateHeightForGroup(update.GroupID, update.BlockHeight)
	}
}
