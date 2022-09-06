package checks

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
)

const (
	periodicHealthCheckInterval = 5 * time.Second
)

type UpstreamStatus struct {
	blockHeightCheck BlockHeightChecker
	peerCheck        Checker
	syncingCheck     Checker
	ID               string
}

// Provide the max block height found across node providers.
func (s *UpstreamStatus) isHealthy(maxBlockHeight uint64) bool {
	if !s.peerCheck.IsPassing() || !s.syncingCheck.IsPassing() || !s.blockHeightCheck.IsPassing(maxBlockHeight) {
		zap.L().Debug("Upstream identifed as unhealthy.", zap.String("upstreamID", s.ID), zap.String("upstreamStatus", fmt.Sprintf("%+v", s)))

		return false
	}

	zap.L().Debug("Upstream identifed as healthy.", zap.String("upstreamID", s.ID), zap.String("upstreamStatus", fmt.Sprintf("%+v", s)))

	return true
}

//go:generate mockery --output ../mocks --name HealthCheckManager
type HealthCheckManager interface {
	StartHealthChecks()
	GetHealthyUpstreams(candidateUpstreams []string) []string
}

type healthCheckManager struct {
	upstreamIDToStatus  map[string]*UpstreamStatus
	ethClientGetter     client.EthClientGetter
	newBlockHeightCheck func(config *conf.UpstreamConfig, clientGetter client.EthClientGetter) BlockHeightChecker
	newPeerCheck        func(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter) Checker
	newSyncingCheck     func(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter) Checker
	configs             []conf.UpstreamConfig
}

func NewHealthCheckManager(ethClientGetter client.EthClientGetter, config []conf.UpstreamConfig) HealthCheckManager {
	return &healthCheckManager{
		upstreamIDToStatus:  make(map[string]*UpstreamStatus),
		ethClientGetter:     ethClientGetter,
		configs:             config,
		newBlockHeightCheck: NewBlockHeightChecker,
		newPeerCheck:        NewPeerChecker,
		newSyncingCheck:     NewSyncingChecker,
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
		if h.upstreamIDToStatus[upstreamID].blockHeightCheck.GetError() == nil && h.upstreamIDToStatus[upstreamID].blockHeightCheck.GetBlockHeight() > maxBlockHeight {
			maxBlockHeight = h.upstreamIDToStatus[upstreamID].blockHeightCheck.GetBlockHeight()
		}
	}

	healthyUpstreams := make([]string, 0)

	for _, upstreamID := range candidateUpstreams {
		if h.upstreamIDToStatus[upstreamID].isHealthy(maxBlockHeight) {
			healthyUpstreams = append(healthyUpstreams, upstreamID)
		}
	}

	zap.L().Debug("Determined currently healthy upstreams.", zap.Any("healthyUpstreams", healthyUpstreams), zap.Any("candidateUpstreams", candidateUpstreams))

	return healthyUpstreams
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

			var blockHeightCheck BlockHeightChecker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				blockHeightCheck = h.newBlockHeightCheck(&config, client.NewEthClient)
			}()

			var peerCheck Checker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				peerCheck = h.newPeerCheck(&config, client.NewEthClient)
			}()

			var syncingCheck Checker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				syncingCheck = h.newSyncingCheck(&config, client.NewEthClient)
			}()

			innerWG.Wait()

			mutex.Lock()
			h.upstreamIDToStatus[config.ID] = &UpstreamStatus{
				ID:               config.ID,
				blockHeightCheck: blockHeightCheck,
				peerCheck:        peerCheck,
				syncingCheck:     syncingCheck,
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

			go func(c BlockHeightChecker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].blockHeightCheck)

			wg.Add(1)

			go func(c Checker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].peerCheck)

			wg.Add(1)

			go func(c Checker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].syncingCheck)
		}

		wg.Wait()

		time.Sleep(periodicHealthCheckInterval)
	}
}
