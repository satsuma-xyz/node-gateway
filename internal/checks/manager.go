package checks

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/types"

	"go.uber.org/zap"

	"github.com/satsuma-data/node-gateway/internal/client"
	conf "github.com/satsuma-data/node-gateway/internal/config"
)

const (
	PeriodicHealthCheckInterval = 5 * time.Second
)

type NewBlockHeightCheck func(
	config *conf.UpstreamConfig,
	clientGetter client.EthClientGetter,
	blockHeightObserver BlockHeightObserver,
	container *metrics.Container,
) types.BlockHeightChecker

//go:generate mockery --output ../mocks --name HealthCheckManager --with-expecter
type HealthCheckManager interface {
	StartHealthChecks()
	IsInitialized() bool
	GetUpstreamStatus(upstreamID string) *types.UpstreamStatus
	RecordRequest(upstreamID string, data *types.RequestData)
}

type healthCheckManager struct {
	blockHeightObserver BlockHeightObserver
	newPeerCheck        func(
		*conf.UpstreamConfig,
		client.EthClientGetter,
		*metrics.Container,
		*zap.Logger,
	) types.Checker
	newBlockHeightCheck func(
		*conf.UpstreamConfig,
		client.EthClientGetter,
		BlockHeightObserver,
		*metrics.Container,
		*zap.Logger,
	) types.BlockHeightChecker
	upstreamIDToStatus map[string]*types.UpstreamStatus
	newSyncingCheck    func(
		*conf.UpstreamConfig,
		client.EthClientGetter,
		*metrics.Container,
		*zap.Logger,
	) types.Checker
	newLatencyCheck func(
		*conf.UpstreamConfig,
		*conf.RoutingConfig,
		client.EthClientGetter,
		*metrics.Container,
		*zap.Logger,
	) types.LatencyChecker
	ethClientGetter     client.EthClientGetter
	healthCheckTicker   *time.Ticker
	metricsContainer    *metrics.Container
	logger              *zap.Logger
	globalRoutingConfig conf.RoutingConfig
	routingConfig       conf.RoutingConfig
	configs             []conf.UpstreamConfig
	isInitialized       atomic.Bool
}

func NewHealthCheckManager(
	ethClientGetter client.EthClientGetter,
	config []conf.UpstreamConfig,
	routingConfig conf.RoutingConfig,
	globalRoutingConfig conf.RoutingConfig,
	blockHeightObserver BlockHeightObserver,
	healthCheckTicker *time.Ticker,
	metricsContainer *metrics.Container,
	logger *zap.Logger,
) HealthCheckManager {
	return &healthCheckManager{
		upstreamIDToStatus:  make(map[string]*types.UpstreamStatus),
		ethClientGetter:     ethClientGetter,
		configs:             config,
		routingConfig:       routingConfig,
		globalRoutingConfig: globalRoutingConfig,
		newBlockHeightCheck: NewBlockHeightChecker,
		newPeerCheck:        NewPeerChecker,
		newSyncingCheck:     NewSyncingChecker,
		newLatencyCheck:     NewLatencyChecker,
		blockHeightObserver: blockHeightObserver,
		healthCheckTicker:   healthCheckTicker,
		metricsContainer:    metricsContainer,
		logger:              logger,
	}
}

func (h *healthCheckManager) StartHealthChecks() {
	h.logger.Info("Starting health checks.")

	go func() {
		h.initializeChecks()
		h.runPeriodicChecks()
	}()
}

func (h *healthCheckManager) GetUpstreamStatus(upstreamID string) *types.UpstreamStatus {
	if status, ok := h.upstreamIDToStatus[upstreamID]; ok {
		return status
	}

	// Panic because an unknown upstream ID implies a bug in the code.
	panic(fmt.Sprintf("Upstream ID %s not found!", upstreamID))
}

func (h *healthCheckManager) RecordRequest(upstreamID string, data *types.RequestData) {
	h.GetUpstreamStatus(upstreamID).LatencyCheck.RecordRequest(data)
}

func (h *healthCheckManager) setUpstreamStatus(upstreamID string, status *types.UpstreamStatus) {
	h.upstreamIDToStatus[upstreamID] = status
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

				blockHeightCheck = h.newBlockHeightCheck(
					&config,
					client.NewEthClient,
					h.blockHeightObserver,
					h.metricsContainer,
					h.logger,
				)
			}()

			var peerCheck types.Checker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				peerCheck = h.newPeerCheck(
					&config,
					client.NewEthClient,
					h.metricsContainer,
					h.logger,
				)
			}()

			var syncingCheck types.Checker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				syncingCheck = h.newSyncingCheck(
					&config,
					client.NewEthClient,
					h.metricsContainer,
					h.logger,
				)
			}()

			var latencyCheck types.LatencyChecker

			innerWG.Add(1)

			go func() {
				defer innerWG.Done()

				latencyCheck = h.newLatencyCheck(
					&config,
					&h.routingConfig,
					client.NewEthClient,
					h.metricsContainer,
					h.logger,
				)
			}()

			innerWG.Wait()

			mutex.Lock()
			h.setUpstreamStatus(config.ID, &types.UpstreamStatus{
				ID:               config.ID,
				GroupID:          config.GroupID,
				BlockHeightCheck: blockHeightCheck,
				PeerCheck:        peerCheck,
				SyncingCheck:     syncingCheck,
				LatencyCheck:     latencyCheck,
			})
			mutex.Unlock()
		}()
	}

	outerWG.Wait()
}

func (h *healthCheckManager) runPeriodicChecks() {
	h.runChecksOnce()

	for range h.healthCheckTicker.C {
		h.runChecksOnce()
	}
}

func (h *healthCheckManager) runChecksOnce() {
	var wg sync.WaitGroup

	for i := range h.configs {
		config := h.configs[i]
		h.logger.Debug("Running healthchecks on upstream.", zap.String("upstreamID", config.ID))

		wg.Add(1)

		go func(c types.BlockHeightChecker) {
			defer wg.Done()
			c.RunCheck()
		}(h.GetUpstreamStatus(config.ID).BlockHeightCheck)

		wg.Add(1)

		go func(c types.Checker) {
			defer wg.Done()
			c.RunCheck()
		}(h.GetUpstreamStatus(config.ID).PeerCheck)

		wg.Add(1)

		go func(c types.Checker) {
			defer wg.Done()
			c.RunCheck()
		}(h.GetUpstreamStatus(config.ID).SyncingCheck)

		wg.Add(1)

		go func(c types.LatencyChecker) {
			defer wg.Done()
			c.RunPassiveCheck()
		}(h.GetUpstreamStatus(config.ID).LatencyCheck)
	}

	wg.Wait()

	h.isInitialized.Store(true)
}

func (h *healthCheckManager) IsInitialized() bool {
	return h.isInitialized.Load()
}
