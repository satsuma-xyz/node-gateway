package types

import (
	"fmt"

	"go.uber.org/zap"
)

type UpstreamStatus struct {
	BlockHeightCheck BlockHeightChecker
	PeerCheck        Checker
	SyncingCheck     Checker
	ID               string
	GroupID          string
}

// Provide the max block height found across node providers.
func (s *UpstreamStatus) IsHealthy(maxBlockHeight uint64) bool {
	if !s.PeerCheck.IsPassing() || !s.SyncingCheck.IsPassing() || !s.BlockHeightCheck.IsPassing(maxBlockHeight) {
		zap.L().Debug("Upstream identifed as unhealthy.", zap.String("upstreamID", s.ID), zap.String("upstreamStatus", fmt.Sprintf("%+v", s)))

		return false
	}

	zap.L().Debug("Upstream identifed as healthy.", zap.String("upstreamID", s.ID), zap.String("upstreamStatus", fmt.Sprintf("%+v", s)))

	return true
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
