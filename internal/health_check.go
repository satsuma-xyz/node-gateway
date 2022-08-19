package internal

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
)

const (
	periodicHealthCheckInterval = 1 * time.Second
)

type UpstreamStatus struct {
	blockHeightCheck checks.BlockHeightChecker
	peerCheck        checks.Checker
	syncingCheck     checks.Checker
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

//go:generate mockery --output ./mocks --name HealthCheckManager
type HealthCheckManager interface {
	StartHealthChecks()
	GetHealthyUpstreams() []string
}

type healthCheckManager struct {
	upstreamIDToStatus  map[string]*UpstreamStatus
	ethClientGetter     client.EthClientGetter
	newBlockHeightCheck func(config *conf.UpstreamConfig, clientGetter client.EthClientGetter) checks.BlockHeightChecker
	newPeerCheck        func(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter) checks.Checker
	newSyncingCheck     func(upstreamConfig *conf.UpstreamConfig, clientGetter client.EthClientGetter) checks.Checker
	configs             []conf.UpstreamConfig
}

func NewHealthCheckManager(ethClientGetter client.EthClientGetter, config []conf.UpstreamConfig) HealthCheckManager {
	return &healthCheckManager{
		upstreamIDToStatus:  make(map[string]*UpstreamStatus),
		ethClientGetter:     ethClientGetter,
		configs:             config,
		newBlockHeightCheck: checks.NewBlockHeightChecker,
		newPeerCheck:        checks.NewPeerChecker,
		newSyncingCheck:     checks.NewSyncingChecker,
	}
}

func (h *healthCheckManager) StartHealthChecks() {
	zap.L().Info("Starting health checks.")

	for i := range h.configs {
		config := h.configs[i]
		blockHeightCheck := h.newBlockHeightCheck(&config, client.NewEthClient)
		peerCheck := h.newPeerCheck(&config, client.NewEthClient)
		syncingCheck := h.newSyncingCheck(&config, client.NewEthClient)

		h.upstreamIDToStatus[config.ID] = &UpstreamStatus{
			ID:               config.ID,
			blockHeightCheck: blockHeightCheck,
			peerCheck:        peerCheck,
			syncingCheck:     syncingCheck,
		}
	}

	go h.runPeriodicChecks(h.configs)
}

func (h *healthCheckManager) GetHealthyUpstreams() []string {
	zap.L().Debug("Determining healthy upstreams.", zap.String("upstreamIDToStatus", fmt.Sprintf("%v", h.upstreamIDToStatus)))

	var maxBlockHeight uint64 = 0

	for _, upstreamStatus := range h.upstreamIDToStatus {
		if upstreamStatus.blockHeightCheck.GetBlockHeight() > maxBlockHeight {
			maxBlockHeight = upstreamStatus.blockHeightCheck.GetBlockHeight()
		}
	}

	healthyUpstreams := make([]string, 0)

	for upstreamID, upstreamStatus := range h.upstreamIDToStatus {
		if upstreamStatus.isHealthy(maxBlockHeight) {
			healthyUpstreams = append(healthyUpstreams, upstreamID)
		}
	}

	zap.L().Debug("Determined currently healthy upstreams.", zap.String("healthyUpstreams", fmt.Sprintf("%v", healthyUpstreams)))

	return healthyUpstreams
}

func (h *healthCheckManager) runPeriodicChecks(configs []conf.UpstreamConfig) {
	for {
		var wg sync.WaitGroup

		for i := range configs {
			config := configs[i]
			zap.L().Debug("Running healthchecks on config", zap.String("config", fmt.Sprintf("%v", config)))

			wg.Add(1)

			go func(c checks.BlockHeightChecker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].blockHeightCheck)

			wg.Add(1)

			go func(c checks.Checker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].peerCheck)

			wg.Add(1)

			go func(c checks.Checker) {
				defer wg.Done()
				c.RunCheck()
			}(h.upstreamIDToStatus[config.ID].syncingCheck)
		}

		wg.Wait()

		time.Sleep(periodicHealthCheckInterval)
	}
}
