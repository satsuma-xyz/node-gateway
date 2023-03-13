package metadata

import "github.com/samber/lo"

type BlockHeightStatus struct {
	Error                error
	GroupID              string
	UpstreamID           string
	BlockHeight          uint64
	GroupMaxBlockHeight  uint64
	GlobalMaxBlockHeight uint64
}

type ChainMetadataStore struct {
	opChannel          chan func()
	maxHeightByGroupID map[string]uint64
	heightByUpstreamID map[string]uint64
	errorByUpstreamID  map[string]error
	globalMaxHeight    uint64
}

func NewChainMetadataStore() *ChainMetadataStore {
	return &ChainMetadataStore{
		maxHeightByGroupID: make(map[string]uint64),
		heightByUpstreamID: make(map[string]uint64),
		errorByUpstreamID:  make(map[string]error),
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
	c.maxHeightByGroupID[groupID] = lo.Max([]uint64{c.maxHeightByGroupID[groupID], currentBlockHeight})
}

func (c *ChainMetadataStore) updateHeightForUpstream(groupID, upstreamID string, blockHeight uint64) {
	c.heightByUpstreamID[groupID+upstreamID] = blockHeight
}

func (c *ChainMetadataStore) updateErrorForUpstream(groupID, upstreamID string, err error) {
	c.errorByUpstreamID[groupID+upstreamID] = err
}

func (c *ChainMetadataStore) GetBlockHeightStatus(groupID, upstreamID string) BlockHeightStatus {
	returnChannel := make(chan BlockHeightStatus)
	c.opChannel <- func() {
		blockHeightStatus := BlockHeightStatus{
			Error:                c.errorByUpstreamID[groupID+upstreamID],
			GroupID:              groupID,
			UpstreamID:           upstreamID,
			BlockHeight:          c.heightByUpstreamID[groupID+upstreamID],
			GroupMaxBlockHeight:  c.maxHeightByGroupID[groupID],
			GlobalMaxBlockHeight: c.globalMaxHeight,
		}
		returnChannel <- blockHeightStatus
		close(returnChannel)
	}

	return <-returnChannel
}

func (c *ChainMetadataStore) ProcessBlockHeightUpdate(groupID, updateID string, blockHeight uint64) {
	c.opChannel <- func() {
		c.globalMaxHeight = lo.Max([]uint64{c.globalMaxHeight, blockHeight})
		c.updateHeightForGroup(groupID, blockHeight)
		c.updateHeightForUpstream(groupID, updateID, blockHeight)
		c.updateErrorForUpstream(groupID, updateID, nil)
	}
}

func (c *ChainMetadataStore) ProcessErrorUpdate(groupID, updateID string, err error) {
	c.opChannel <- func() {
		c.updateErrorForUpstream(groupID, updateID, err)
	}
}
