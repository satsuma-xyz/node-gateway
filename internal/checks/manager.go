package checks

import (
	"fmt"
	"github.com/satsuma-data/node-gateway/internal/types"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
)

const (
	periodicHealthCheckInterval = 5 * time.Second
)

//go:generate mockery --output ../mocks --name HealthCheckManager --with-expecter
type HealthCheckManager interface {
	StartHealthChecks()
	GetHealthyUpstreams(candidateUpstreams []string) []string
	GetUpstreamStatus(upstreamId string) *types.UpstreamStatus
}

type healthCheckManager struct {
	upstreamIDToStatus  map[string]*types.UpstreamStatus
	ethClientGetter     client.EthClientGetter
	newBlockHeightCheck func(config *conf.UpstreamConfig, clientGetter client.EthClientGetter, blockHeightObserver chan<- uint64) types.BlockHeightChecker
	newPeerCheck        func(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter) types.Checker
	newSyncingCheck     func(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter) types.Checker
	configs             []conf.UpstreamConfig
	blockHeightObserver chan<- uint64
}

func NewHealthCheckManager(ethClientGetter client.EthClientGetter, config []conf.UpstreamConfig, blockHeightObserver chan<- uint64) HealthCheckManager {
	return &healthCheckManager{
		upstreamIDToStatus:  make(map[string]*types.UpstreamStatus),
		ethClientGetter:     ethClientGetter,
		configs:             config,
		newBlockHeightCheck: NewBlockHeightChecker,
		newPeerCheck:        NewPeerChecker,
		newSyncingCheck:     NewSyncingChecker,
		blockHeightObserver: blockHeightObserver,
	}
}

func (h *healthCheckManager) StartHealthChecks() {
	zap.L().Info("Starting health checks.")

	go func() {
		h.initializeChecks()
		h.runPeriodicChecks()
	}()
}

func (h *healthCheckManager) GetHealthyUpstreams(candidateUpstreams []string) []string {
	zap.L().Debug("Determining healthy upstreams from candidates.", zap.Any("candidateUpstreams", candidateUpstreams))

	var maxBlockHeight uint64 = 0

	for _, upstreamID := range candidateUpstreams {
		if h.upstreamIDToStatus[upstreamID].BlockHeightCheck.GetError() == nil && h.upstreamIDToStatus[upstreamID].BlockHeightCheck.GetBlockHeight() > maxBlockHeight {
			maxBlockHeight = h.upstreamIDToStatus[upstreamID].BlockHeightCheck.GetBlockHeight()
		}
	}

	healthyUpstreams := make([]string, 0)

	for _, upstreamID := range candidateUpstreams {
		if h.upstreamIDToStatus[upstreamID].IsHealthy(maxBlockHeight) {
			healthyUpstreams = append(healthyUpstreams, upstreamID)
		}
	}

	zap.L().Debug("Determined currently healthy upstreams.", zap.Any("healthyUpstreams", healthyUpstreams), zap.Any("candidateUpstreams", candidateUpstreams))

	return healthyUpstreams
}

func (h *healthCheckManager) GetUpstreamStatus(upstreamId string) *types.UpstreamStatus {
	if status, ok := h.upstreamIDToStatus[upstreamId]; ok {
		return status
	} else {
		// Panic because an unknown upstream ID implies a bug in the code.
		panic(fmt.Sprintf("Upstream ID %s not found!", upstreamId))
	}
}

func (h *healthCheckManager) initializeChecks() {
	var mutex sync.RWMutex

	var outerWG sync.WaitGroup

	// Parallelize to speed up gateway startup.
	for i := range h.configs {
		config := h.configs[i]

		outerWG.Add(1)

		go func() {
			defer outerWG.Done()

			var innerWG sync.WaitGroup

			var blockHeightCheck types.BlockHeightChecker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				blockHeightCheck = h.newBlockHeightCheck(&config, client.NewEthClient, h.blockHeightObserver)
			}()

			var peerCheck types.Checker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				peerCheck = h.newPeerCheck(&config, client.NewEthClient)
			}()

			var syncingCheck types.Checker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				syncingCheck = h.newSyncingCheck(&config, client.NewEthClient)
			}()

			innerWG.Wait()

			mutex.Lock()
			h.upstreamIDToStatus[config.ID] = &types.UpstreamStatus{
				ID:               config.ID,
				BlockHeightCheck: blockHeightCheck,
				PeerCheck:        peerCheck,
				SyncingCheck:     syncingCheck,
			}
			mutex.Unlock()
		}()
	}

	outerWG.Wait()
}

func (h *healthCheckManager) runPeriodicChecks() {
	for {
		var wg sync.WaitGroup

		for i := range h.configs {
			config := h.configs[i]
			zap.L().Debug("Running healthchecks on upstream.", zap.String("upstreamID", config.ID))

			wg.Add(1)

			go func(c types.BlockHeightChecker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].BlockHeightCheck)

			wg.Add(1)

			go func(c types.Checker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].PeerCheck)

			wg.Add(1)

			go func(c types.Checker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].SyncingCheck)
		}

		wg.Wait()

		time.Sleep(periodicHealthCheckInterval)
	}
}
