package types

import (
	"time"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

type UpstreamStatus struct {
	BlockHeightCheck BlockHeightChecker
	PeerCheck        Checker
	SyncingCheck     Checker
	LatencyCheck     LatencyChecker
	ID               string
	GroupID          string
}

type RequestData struct {
	ResponseBody     jsonrpc.ResponseBody
	Method           string
	HTTPResponseCode int
	Latency          time.Duration
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

//go:generate mockery --output ../mocks --name LatencyChecker --with-expecter
type LatencyChecker interface {
	RunPassiveCheck()
	GetUnhealthyReason(methods []string) config.UnhealthyReason
	RecordRequest(data *RequestData)
}

type PriorityToUpstreamsMap map[int][]*config.UpstreamConfig
