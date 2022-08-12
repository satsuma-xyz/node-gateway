package internal

import (
	"context"
	"errors"
	"fmt"
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

func (h *HealthCheckManager) StartHealthChecks(configs []UpstreamConfig) {
	zap.L().Info("Starting health checks.")

	for _, config := range configs {
		// Set `isSyncing:true` until we check the node's syncing status
		h.nodeIDToStatus[config.ID] = &NodeStatus{isSyncing: true}
	}

	for _, config := range configs {
		if config.UseWsForBlockHeight {
			// TODO: handle case of subscribe failure, fall back to using HTTP polling
			go h.monitorMaxBlockHeightByWebsocket(config.ID, config.WSURL)
		}
	}

	go h.runPeriodicChecks(configs)
}

// Tolerate being at most 4 blocks behind the max block height across nodes.
// We should think about making this configurable in the config file.
const HealthyBlockHeightBelowMax = 4

func (h *HealthCheckManager) GetCurrentHealthyNodes() map[string]*NodeStatus {
	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	zap.L().Debug("Currently determining healthy nodes.", zap.String("nodeIDToStatus", fmt.Sprintf("%v", h.nodeIDToStatus)))

	healthyNodes := make(map[string]*NodeStatus)

	// Exclude nodes that have errors or still syncing
	var maxBlockHeight uint64 = 0

	for nodeID, nodeStatus := range h.nodeIDToStatus {
		if nodeStatus.connectionError != nil ||
			nodeStatus.getCurrentBlockNumberError != nil ||
			nodeStatus.getPeerCountError != nil ||
			nodeStatus.getIsSyncingError != nil {
			zap.L().Debug("Node experienced errors in healthchecks, marking it as unhealthy.", zap.String("nodeID", nodeID), zap.String("nodeStatus", fmt.Sprintf("%v", nodeStatus)))
			continue
		}

		if nodeStatus.isSyncing {
			zap.L().Debug("Node is still syncing, marking it is unhealthy.", zap.String("nodeID", nodeID), zap.String("nodeStatus", fmt.Sprintf("%v", nodeStatus)))
			continue
		}

		if nodeStatus.currentBlockNumber > maxBlockHeight {
			maxBlockHeight = nodeStatus.currentBlockNumber
		}

		healthyNodes[nodeID] = nodeStatus
	}

	for nodeID, nodeStatus := range healthyNodes {
		if maxBlockHeight-nodeStatus.currentBlockNumber > HealthyBlockHeightBelowMax {
			delete(healthyNodes, nodeID)
		}
	}

	zap.L().Debug("Deteremined currently healthy nodes.", zap.String("healthyNodes", fmt.Sprintf("%v", healthyNodes)))

	return healthyNodes
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

func (h *HealthCheckManager) runPeriodicChecks(configs []UpstreamConfig) {
	for {
		for _, config := range configs {
			zap.L().Debug("Running healthchecks on config", zap.String("config", fmt.Sprintf("%v", config)))

			httpClient, err := h.ethClientGetter(config.HTTPURL)
			if err != nil {
				zap.L().Error("Failed to connect to node to run periodic checks.", zap.String("nodeID", config.ID), zap.Error(err))
				h.statusMutex.Lock()
				h.nodeIDToStatus[config.ID].connectionError = err
				h.statusMutex.Unlock()

				continue
			}

			// Get the max block height if we're not doing `eth_subscribe` to `newHeads` via websockets
			if !config.UseWsForBlockHeight {
				go h.checkMaxBlockHeightByHTTP(config.ID, httpClient)
			}

			go h.checkPeerCount(config.ID, httpClient)
			go h.checkIsNodeSyncing(config.ID, httpClient)
		}

		time.Sleep(PeriodicHealthCheckInterval)
	}
}

func (h *HealthCheckManager) checkMaxBlockHeightByHTTP(nodeID string, httpClient EthClient) {
	header, err := httpClient.HeaderByNumber(context.Background(), nil)

	zap.L().Debug("Running checkMaxBlockHeightByHTTP on config", zap.Any("nodeID", nodeID), zap.Any("response", header), zap.Error(err))

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

	zap.L().Debug("Running checkPeerCount on config", zap.Any("nodeID", nodeID), zap.Any("response", peerCount), zap.Error(err))

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

	zap.L().Debug("Running checkIsNodeSyncing on config", zap.Any("nodeID", nodeID), zap.Any("response", syncProgress), zap.Error(err))

	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	if err != nil {
		h.nodeIDToStatus[nodeID].getIsSyncingError = err
		return
	}

	h.nodeIDToStatus[nodeID].isSyncing = syncProgress != nil
}
