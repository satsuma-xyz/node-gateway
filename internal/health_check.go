package internal

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"go.uber.org/zap"
)

const PeriodicHealthCheckInterval = 1 * time.Second

type NodeStatus struct {
	getCurrentBlockNumberError error
	getPeerCountError          error
	getIsSyncingError          error
	connectionError            error
	currentBlockNumber         uint64
	peerCount                  uint64
	isSyncing                  bool
}

type HealthCheckConfig struct {
	nodeID       string
	httpURL      string
	websocketURL string
	// Specifies whether to use a websocket subscription to `newHeads` for monitoring block height. Will fall back to HTTP polling if not set.
	useWsForBlockHeight bool
}

func (c HealthCheckConfig) isValid() bool {
	isValid := true
	if c.httpURL == "" {
		isValid = false

		zap.L().Error("httpUrl cannot be empty", zap.Any("config", c), zap.String("nodeId", c.nodeID))
	}

	if c.useWsForBlockHeight && c.websocketURL == "" {
		isValid = false

		zap.L().Error("websocketUrl should be provided if shouldSubscribeNewHeads=true.", zap.Any("config", c), zap.String("nodeId", c.nodeID))
	}

	return isValid
}

type HealthCheckManager struct {
	nodeIDToStatus  map[string]*NodeStatus
	statusMutex     *sync.RWMutex
	ethClientGetter EthClientGetter
}

func NewHealthCheckManager(ethClientGetter EthClientGetter) *HealthCheckManager {
	return &HealthCheckManager{
		nodeIDToStatus:  make(map[string]*NodeStatus),
		statusMutex:     &sync.RWMutex{},
		ethClientGetter: ethClientGetter,
	}
}

func (h *HealthCheckManager) StartHealthChecks(configs []HealthCheckConfig) {
	zap.L().Info("Starting health checks.")

	for _, config := range configs {
		if !config.isValid() {
			zap.L().Panic("Config not valid.", zap.Any("config", config), zap.String("nodeId", config.nodeID))
		}

		// Set `isSyncing:true` until we check the node's syncing status
		h.nodeIDToStatus[config.nodeID] = &NodeStatus{isSyncing: true}
	}

	for _, config := range configs {
		if config.useWsForBlockHeight {
			// TODO: handle case of subscribe failure, fall back to using HTTP polling
			go h.monitorMaxBlockHeightByWebsocket(config.nodeID, config.websocketURL)
		}
	}

	go h.runPeriodicChecks(configs)
}

func (h *HealthCheckManager) monitorMaxBlockHeightByWebsocket(nodeID, websocketURL string) {
	websocketClient, err := h.ethClientGetter(websocketURL)

	if err != nil {
		zap.L().Error("Failed to connect to node to monitor max block height.", zap.String("nodeID", nodeID), zap.Error(err))
		h.statusMutex.Lock()
		h.nodeIDToStatus[nodeID].getCurrentBlockNumberError = err
		h.statusMutex.Unlock()

		return
	}

	onNewHead := func(header *types.Header) {
		h.statusMutex.Lock()
		h.nodeIDToStatus[nodeID].currentBlockNumber = header.Number.Uint64()
		h.statusMutex.Unlock()
	}

	onError := func(failure string) {
		h.statusMutex.Lock()
		h.nodeIDToStatus[nodeID].getCurrentBlockNumberError = errors.New(failure)
		h.statusMutex.Unlock()
	}

	// TODO: Restarting the WS connection if it fails at some point
	if err = subscribeNewHead(websocketClient, &newHeadHandler{onNewHead: onNewHead, onError: onError}); err != nil {
		zap.L().Error("Failed to subscribe to new head to monitor max block height.", zap.String("nodeID", nodeID), zap.Error(err))
		h.statusMutex.Lock()
		h.nodeIDToStatus[nodeID].getCurrentBlockNumberError = err
		h.statusMutex.Unlock()

		return
	}

	zap.L().Info("Successfully subscribed to new head to monitor max block height.", zap.String("nodeID", nodeID), zap.String("websocketURL", websocketURL))
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

func (h *HealthCheckManager) runPeriodicChecks(configs []HealthCheckConfig) {
	for {
		for _, config := range configs {
			httpClient, err := h.ethClientGetter(config.httpURL)
			if err != nil {
				zap.L().Error("Failed to connect to node to run periodic checks.", zap.String("nodeID", config.nodeID), zap.Error(err))
				h.statusMutex.Lock()
				h.nodeIDToStatus[config.nodeID].connectionError = err
				h.statusMutex.Unlock()

				continue
			}

			// Get the max block height if we're not doing `eth_subscribe` to `newHeads` via websockets
			if !config.useWsForBlockHeight {
				go h.checkMaxBlockHeightByHTTP(config.nodeID, httpClient)
			}

			go h.checkPeerCount(config.nodeID, httpClient)
			go h.checkIsNodeSyncing(config.nodeID, httpClient)
		}

		time.Sleep(PeriodicHealthCheckInterval)
	}
}

func (h *HealthCheckManager) checkMaxBlockHeightByHTTP(nodeID string, httpClient EthClient) {
	header, err := httpClient.HeaderByNumber(context.Background(), nil)

	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	if err != nil {
		h.nodeIDToStatus[nodeID].getCurrentBlockNumberError = err
		return
	}

	h.nodeIDToStatus[nodeID].currentBlockNumber = header.Number.Uint64()
}

func (h *HealthCheckManager) checkPeerCount(nodeID string, httpClient EthClient) {
	peerCount, err := httpClient.PeerCount(context.Background())

	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	if err != nil {
		h.nodeIDToStatus[nodeID].getPeerCountError = err
		return
	}

	h.nodeIDToStatus[nodeID].peerCount = peerCount
}

func (h *HealthCheckManager) checkIsNodeSyncing(nodeID string, httpClient EthClient) {
	syncProgress, err := httpClient.SyncProgress(context.Background())

	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	if err != nil {
		h.nodeIDToStatus[nodeID].getIsSyncingError = err
		return
	}

	h.nodeIDToStatus[nodeID].isSyncing = syncProgress != nil
}
