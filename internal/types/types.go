package types

import (
	"github.com/satsuma-data/node-gateway/internal/config"
)

type UpstreamStatus struct {
	BlockHeightCheck BlockHeightChecker
	PeerCheck        Checker
	SyncingCheck     Checker
	LatencyCheck     Checker
	ID               string
	GroupID          string
}

//go:generate mockery --output ../mocks --name BlockHeightChecker --with-expecter
type BlockHeightChecker interface {
	RunCheck()
	GetError() error
	GetBlockHeight() uint64
	IsPassing(maxBlockHeight uint64) bool
}

//go:generate mockery --output ../mocks --name Checker --with-expecter
type Checker interface {
	RunCheck()
	IsPassing() bool
}

type PriorityToUpstreamsMap map[int][]*config.UpstreamConfig
