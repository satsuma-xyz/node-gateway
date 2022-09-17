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
	go func() {
		for op := range c.opChannel {
			op()
		}
	}()
}

func (c *ChainMetadataStore) updateHeightForGroup(groupID string, currentBlockHeight uint64) {
	c.maxHeightByGroupID[groupID] = max(c.maxHeightByGroupID[groupID], currentBlockHeight)
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

	return <-returnChannel
}

func (c *ChainMetadataStore) GetMaxHeightForGroup(groupID string) uint64 {
	var returnChannel = make(chan uint64)
	c.opChannel <- func() {
		returnChannel <- c.maxHeightByGroupID[groupID]
		close(returnChannel)
	}

	return <-returnChannel
}

func (c *ChainMetadataStore) ProcessUpdate(update BlockHeightUpdate) {
	c.opChannel <- func() {
		c.globalMaxHeight = max(c.globalMaxHeight, update.BlockHeight)
		c.updateHeightForGroup(update.GroupID, update.BlockHeight)
	}
}
