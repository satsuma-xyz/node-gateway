package metadata

type ChainMetadataStore struct {
	blockHeightChannel <-chan uint64
	maxHeight          uint64
}

func NewChainMetadataStore(blockHeightChannel <-chan uint64) *ChainMetadataStore {
	return &ChainMetadataStore{blockHeightChannel: blockHeightChannel}
}

func (c *ChainMetadataStore) Start() {
	go func() {
		for {
			var currentBlockHeight = <-c.blockHeightChannel
			c.maxHeight = max(c.maxHeight, currentBlockHeight)
		}
	}()
}

func max(a, b uint64) uint64 {
	if a > b {
		return a
	}

	return b
}

func (c *ChainMetadataStore) GetMaxHeight() uint64 {
	return c.maxHeight
}
