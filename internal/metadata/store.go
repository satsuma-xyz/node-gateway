package metadata

type BlockHeightUpdate struct {
	GroupID     string
	BlockHeight uint64
}

type ReadRequest struct {
	isGlobal bool
	groupID  string
}

type ChainMetadataStore struct {
	blockHeightChannel  <-chan BlockHeightUpdate
	maxHeightByGroupID  map[string]uint64
	globalMaxHeight     uint64
	readRequestChannel  chan ReadRequest
	readResponseChannel chan uint64
	opChannel           chan func()
}

func NewChainMetadataStore(blockHeightChannel <-chan BlockHeightUpdate) *ChainMetadataStore {
	return &ChainMetadataStore{
		blockHeightChannel: blockHeightChannel,
		maxHeightByGroupID: make(map[string]uint64),
		opChannel:          make(chan func()),
	}
}

func (c *ChainMetadataStore) Start() {
	go func() {
		for update := range c.blockHeightChannel {
			c.ProcessUpdate(update)
		}
	}()
	go func(c *ChainMetadataStore) {
		for op := range c.opChannel {
			op()
		}
		//for {
		//	select {
		//	//case update := <-c.blockHeightChannel:
		//	//	c.ProcessUpdate(update)
		//	case op := <-c.opChannel:
		//		op()
		//	}
		//}
		//for r := range c.opChan {
		//	r.fn()
		//}
		//for {
		//	select {
		//	case update := <-c.blockHeightChannel:
		//		c.globalMaxHeight = max(c.globalMaxHeight, update.BlockHeight)
		//		c.updateHeightForGroup(update.GroupID, update.BlockHeight)
		//	case readRequest := <-c.readRequestChannel:
		//		if readRequest.isGlobal {
		//			c.readResponseChannel <- c.globalMaxHeight
		//		} else {
		//			c.readResponseChannel <- c.maxHeightByGroupID[readRequest.groupID]
		//		}
		//	}
		//	//var update = <-c.blockHeightChannel
		//	//c.globalMaxHeight = max(c.globalMaxHeight, update.BlockHeight)
		//	////atomic.
		//	//c.updateHeightForGroup(update.GroupID, update.BlockHeight)
		//}
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
