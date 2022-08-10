package internal

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type EthClient interface {
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	PeerCount(ctx context.Context) (uint64, error)
	SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error)
}

func NewEthClient(url string) (EthClient, error) {
	c, err := ethclient.Dial(url)
	if err != nil {
		return nil, err
	}

	return c, nil
}
