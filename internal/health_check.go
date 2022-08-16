package internal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"go.uber.org/zap"
)

const (
	periodicHealthCheckInterval  = 1 * time.Second
	minimumPeerCount             = 5
	JSONRPCErrCodeMethodNotFound = -32601
)

func isMethodNotSupportedErr(err error) bool {
	if err == nil {
		return false
	}

	switch e := err.(type) {
	case rpc.Error:
		return e.ErrorCode() == JSONRPCErrCodeMethodNotFound
	default:
		return false
	}
}

type UpstreamStatus struct {
	currentBlockNumberError error
	peerCountError          error
	peerCheck               error
	isSyncingError          error
	connectionError         error
	ID                      string
	currentBlockNumber      uint64
	peerCount               uint64
	isSyncing               bool
}

// Provide the max block height found across node providers.
func (s *UpstreamStatus) isHealthy(maxBlockHeight uint64) bool {
	if s.connectionError != nil ||
		s.currentBlockNumberError != nil ||
		s.peerCountError != nil && !isMethodNotSupportedErr(s.peerCountError) ||
		s.isSyncingError != nil && !isMethodNotSupportedErr(s.isSyncingError) {
		zap.L().Debug("Upstream experienced errors in healthchecks, marking it as unhealthy.", zap.String("upstreamID", s.ID), zap.String("upstreamStatus", fmt.Sprintf("%v", s)))
		return false
	}

	if !isMethodNotSupportedErr(s.isSyncingError) && s.isSyncing {
		zap.L().Debug("Upstream is still syncing, marking it as unhealthy.", zap.String("upstreamID", s.ID), zap.String("upstreamStatus", fmt.Sprintf("%v", s)))
		return false
	}

	if s.peerCount < minimumPeerCount {
		zap.L().Debug("Upstream beneath the minimum peer count, marking it is unhealthy.", zap.String("upstreamID", s.ID), zap.Int("minPeerCount", minimumPeerCount), zap.String("upstreamStatus", fmt.Sprintf("%v", s)))
		return false
	}

	if s.currentBlockNumber < maxBlockHeight {
		zap.L().Debug("Upstream beneath the max block height found across upstream providers, marking it as unhealthy.", zap.String("upstreamID", s.ID), zap.Int("maxBlockHeight", int(maxBlockHeight)), zap.Int("blockHeight", int(s.currentBlockNumber)), zap.String("upstreamStatus", fmt.Sprintf("%v", s)))
		return false
	}

	zap.L().Debug("Upstream identifed as healthy upstream.", zap.String("upstreamID", s.ID), zap.String("upstreamStatus", fmt.Sprintf("%v", s)))

	return true
}

type HealthCheckManager struct {
	upstreamIDToStatus map[string]*UpstreamStatus
	statusMutex        *sync.RWMutex
	ethClientGetter    EthClientGetter
	configs            []UpstreamConfig
}

func NewHealthCheckManager(ethClientGetter EthClientGetter, config []UpstreamConfig) *HealthCheckManager {
	return &HealthCheckManager{
		upstreamIDToStatus: make(map[string]*UpstreamStatus),
		statusMutex:        &sync.RWMutex{},
		ethClientGetter:    ethClientGetter,
		configs:            config,
	}
}

func (h *HealthCheckManager) StartHealthChecks() {
	zap.L().Info("Starting health checks.")

	for _, config := range h.configs {
		// Set `isSyncing:true` until we check the upstream node's syncing status
		h.upstreamIDToStatus[config.ID] = &UpstreamStatus{
			ID:        config.ID,
			isSyncing: true,
		}
	}

	for _, config := range h.configs {
		if shouldUseWSForBlockHeight(config) {
			// :TODO: handle case of subscribe failure, fall back to using HTTP polling
			go h.monitorMaxBlockHeightByWebsocket(config.ID, config.WSURL)
		}
	}

	go h.runPeriodicChecks(h.configs)
}

func shouldUseWSForBlockHeight(config UpstreamConfig) bool {
	if config.WSURL != "" {
		if config.HealthCheckConfig.UseWSForBlockHeight == nil || *config.HealthCheckConfig.UseWSForBlockHeight {
			return true
		}
	}

	return false
}

func (h *HealthCheckManager) GetHealthyUpstreams() []string {
	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	zap.L().Debug("Determining healthy upstreams.", zap.String("upstreamIDToStatus", fmt.Sprintf("%v", h.upstreamIDToStatus)))

	var maxBlockHeight uint64 = 0

	for _, upstreamStatus := range h.upstreamIDToStatus {
		if upstreamStatus.currentBlockNumber > maxBlockHeight {
			maxBlockHeight = upstreamStatus.currentBlockNumber
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

func (h *HealthCheckManager) monitorMaxBlockHeightByWebsocket(upstreamID, websocketURL string) {
	zap.L().Debug("Monitoring max block height for upstream via websockets", zap.String("upstreamID", upstreamID), zap.String("websocketURL", websocketURL))
	websocketClient, err := h.ethClientGetter(websocketURL)

	if err != nil {
		zap.L().Error("Failed to connect to upstream to monitor max block height.", zap.String("upstreamID", upstreamID), zap.Error(err))
		h.statusMutex.Lock()
		h.upstreamIDToStatus[upstreamID].currentBlockNumberError = err
		h.statusMutex.Unlock()

		return
	}

	onNewHead := func(header *types.Header) {
		h.statusMutex.Lock()
		h.upstreamIDToStatus[upstreamID].currentBlockNumber = header.Number.Uint64()
		h.upstreamIDToStatus[upstreamID].currentBlockNumberError = nil
		h.statusMutex.Unlock()
	}

	onError := func(failure string) {
		h.statusMutex.Lock()
		h.upstreamIDToStatus[upstreamID].currentBlockNumberError = errors.New(failure)
		h.statusMutex.Unlock()
	}

	// :TODO: Restarting the WS connection if it fails at some point
	if err = subscribeNewHead(websocketClient, &newHeadHandler{onNewHead: onNewHead, onError: onError}); err != nil {
		zap.L().Error("Failed to subscribe to new head to monitor max block height.", zap.String("upstreamID", upstreamID), zap.Error(err))
		h.statusMutex.Lock()
		h.upstreamIDToStatus[upstreamID].currentBlockNumberError = err
		h.statusMutex.Unlock()

		return
	}

	zap.L().Info("Successfully subscribed to new head to monitor max block height.", zap.String("upstreamID", upstreamID), zap.String("websocketURL", websocketURL))
}

type newHeadHandler struct {
	onNewHead func(header *types.Header)
	onError   func(failure string)
}

func subscribeNewHead(websocketClient EthClient, handler *newHeadHandler) error {
	ch := make(chan *types.Header)
	subscription, err := websocketClient.SubscribeNewHead(context.Background(), ch)

	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case header := <-ch:
				handler.onNewHead(header)

			case err := <-subscription.Err():
				if err != nil {
					handler.onError(err.Error())
				}

				return
			}
		}
	}()

	return nil
}

func (h *HealthCheckManager) runPeriodicChecks(configs []UpstreamConfig) {
	for {
		for _, config := range configs {
			zap.L().Debug("Running healthchecks on config", zap.String("config", fmt.Sprintf("%v", config)))

			httpClient, err := h.ethClientGetter(config.HTTPURL)
			h.statusMutex.Lock()
			h.upstreamIDToStatus[config.ID].connectionError = err
			h.statusMutex.Unlock()

			if err != nil {
				zap.L().Error("Failed to connect to upstream node to run periodic checks.", zap.String("upstreamID", config.ID), zap.Error(err))
				continue
			}

			// Already doing `eth_subscribe` to `newHeads` via websockets to get max block height
			if !shouldUseWSForBlockHeight(config) {
				go h.checkMaxBlockHeightByHTTP(config.ID, httpClient)
			}

			if !isMethodNotSupportedErr(h.upstreamIDToStatus[config.ID].peerCountError) {
				go h.checkPeerCount(config.ID, httpClient)
			}

			if !isMethodNotSupportedErr(h.upstreamIDToStatus[config.ID].isSyncingError) {
				go h.checkIsUpstreamSyncing(config.ID, httpClient)
			}
		}

		time.Sleep(periodicHealthCheckInterval)
	}
}

func (h *HealthCheckManager) checkMaxBlockHeightByHTTP(upstreamID string, httpClient EthClient) {
	header, err := httpClient.HeaderByNumber(context.Background(), nil)

	zap.L().Debug("Running checkMaxBlockHeightByHTTP on config", zap.Any("upstreamID", upstreamID), zap.Any("response", header), zap.Error(err))

	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	h.upstreamIDToStatus[upstreamID].isSyncingError = err

	if err != nil {
		return
	}

	h.upstreamIDToStatus[upstreamID].currentBlockNumber = header.Number.Uint64()
}

func (h *HealthCheckManager) checkPeerCount(upstreamID string, httpClient EthClient) {
	peerCount, err := httpClient.PeerCount(context.Background())

	zap.L().Debug("Running checkPeerCount on config", zap.Any("upstreamID", upstreamID), zap.Any("response", peerCount), zap.Error(err))

	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	h.upstreamIDToStatus[upstreamID].peerCountError = err

	if err != nil {
		return
	}

	h.upstreamIDToStatus[upstreamID].peerCount = peerCount
}

func (h *HealthCheckManager) checkIsUpstreamSyncing(upstreamID string, httpClient EthClient) {
	syncProgress, err := httpClient.SyncProgress(context.Background())

	zap.L().Debug("Running checkIsUpstreamSyncing on config", zap.Any("upstreamID", upstreamID), zap.Any("response", syncProgress), zap.Error(err))

	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	h.upstreamIDToStatus[upstreamID].isSyncingError = err

	if err != nil {
		return
	}

	h.upstreamIDToStatus[upstreamID].isSyncing = syncProgress != nil
}
