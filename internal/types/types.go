package types

import (
	"time"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
)

type UpstreamStatus struct {
	BlockHeightCheck BlockHeightChecker
	PeerCheck        Checker
	ErrorCheck       ErrorLatencyChecker
	LatencyCheck     ErrorLatencyChecker
	ID               string
	GroupID          string
}

type RequestData struct {
	ResponseBody     jsonrpc.ResponseBody
	Error            error
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

//go:generate mockery --output ../mocks --name ErrorLatencyChecker --with-expecter
type ErrorLatencyChecker interface {
	IsPassing(methods []string) bool
	RecordRequest(data *RequestData) bool
}

type PriorityToUpstreamsMap map[int][]*config.UpstreamConfig
