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

type healthCheckError struct{ err error }

func (h healthCheckError) isMethodNotSupportedErr() bool {
	if h.err == nil {
		return false
	}

	switch e := h.err.(type) {
	case rpc.Error:
		return e.ErrorCode() == JSONRPCErrCodeMethodNotFound
	default:
		return false
	}
}

type NodeStatus struct {
	getCurrentBlockNumberError healthCheckError
	getPeerCountError          healthCheckError
	getIsSyncingError          healthCheckError
	connectionError            healthCheckError
	nodeID                     string
	currentBlockNumber         uint64
	peerCount                  uint64
	isSyncing                  bool
}

type globalState struct {
	maxBlockHeight uint64
}

// Provide the max block height found across node providers.
func (s *NodeStatus) isHealthy(globalState *globalState) bool {
	if s.connectionError.err != nil ||
		s.getCurrentBlockNumberError.err != nil ||
		(s.getPeerCountError.err != nil && !s.getPeerCountError.isMethodNotSupportedErr()) ||
		(s.getIsSyncingError.err != nil && !s.getIsSyncingError.isMethodNotSupportedErr()) {
		zap.L().Debug("Node experienced errors in healthchecks, marking it as unhealthy.", zap.String("nodeID", s.nodeID), zap.String("nodeStatus", fmt.Sprintf("%v", s)))
		return false
	}

	if !s.getIsSyncingError.isMethodNotSupportedErr() && s.isSyncing {
		zap.L().Debug("Node is still syncing, marking it is unhealthy.", zap.String("nodeID", s.nodeID), zap.String("nodeStatus", fmt.Sprintf("%v", s)))
		return false
	}

	if s.peerCount < minimumPeerCount {
		zap.L().Debug("Node beneath the minimum peer count, marking it is unhealthy.", zap.String("nodeID", s.nodeID), zap.Int("minPeerCount", minimumPeerCount), zap.String("nodeStatus", fmt.Sprintf("%v", s)))
		return false
	}

	if s.currentBlockNumber < globalState.maxBlockHeight {
		zap.L().Debug("Node beneath the max block height found across node providers, marking it is unhealthy.", zap.String("nodeID", s.nodeID), zap.Int("maxBlockHeight", int(globalState.maxBlockHeight)), zap.Int("blockHeight", int(s.currentBlockNumber)), zap.String("nodeStatus", fmt.Sprintf("%v", s)))
		return false
	}

	zap.L().Debug("Node identifed as healthy node.", zap.String("nodeID", s.nodeID), zap.String("nodeStatus", fmt.Sprintf("%v", s)))

	return true
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
		h.nodeIDToStatus[config.ID] = &NodeStatus{nodeID: config.ID, isSyncing: true}
	}

	for _, config := range configs {
		if config.HealthCheckConfig.UseWSForBlockHeight {
			// :TODO: handle case of subscribe failure, fall back to using HTTP polling
			go h.monitorMaxBlockHeightByWebsocket(config.ID, config.WSURL)
		}
	}

	go h.runPeriodicChecks(configs)
}

func (h *HealthCheckManager) GetHealthyNodes() []string {
	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	zap.L().Debug("Determining healthy nodes.", zap.String("nodeIDToStatus", fmt.Sprintf("%v", h.nodeIDToStatus)))

	var maxBlockHeight uint64 = 0

	for _, nodeStatus := range h.nodeIDToStatus {
		if nodeStatus.currentBlockNumber > maxBlockHeight {
			maxBlockHeight = nodeStatus.currentBlockNumber
		}
	}

	globalState := &globalState{
		maxBlockHeight: maxBlockHeight,
	}

	healthyNodes := make([]string, 0)

	for nodeID, nodeStatus := range h.nodeIDToStatus {
		if nodeStatus.isHealthy(globalState) {
			healthyNodes = append(healthyNodes, nodeID)
		}
	}

	zap.L().Debug("Determined currently healthy nodes.", zap.String("healthyNodes", fmt.Sprintf("%v", healthyNodes)))

	return healthyNodes
}

func (h *HealthCheckManager) monitorMaxBlockHeightByWebsocket(nodeID, websocketURL string) {
	zap.L().Debug("Monitoring max block height for node via websockets", zap.String("nodeId", nodeID), zap.String("websocketURL", websocketURL))
	websocketClient, err := h.ethClientGetter(websocketURL)

	if err != nil {
		zap.L().Error("Failed to connect to node to monitor max block height.", zap.String("nodeID", nodeID), zap.Error(err))
		h.statusMutex.Lock()
		h.nodeIDToStatus[nodeID].getCurrentBlockNumberError = healthCheckError{err: err}
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
		h.nodeIDToStatus[nodeID].getCurrentBlockNumberError = healthCheckError{err: errors.New(failure)}
		h.statusMutex.Unlock()
	}

	// :TODO: Restarting the WS connection if it fails at some point
	if err = subscribeNewHead(websocketClient, &newHeadHandler{onNewHead: onNewHead, onError: onError}); err != nil {
		zap.L().Error("Failed to subscribe to new head to monitor max block height.", zap.String("nodeID", nodeID), zap.Error(err))
		h.statusMutex.Lock()
		h.nodeIDToStatus[nodeID].getCurrentBlockNumberError = healthCheckError{err: err}
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
				h.nodeIDToStatus[config.ID].connectionError = healthCheckError{err: err}
				h.statusMutex.Unlock()

				continue
			}

			// Node is already doing `eth_subscribe` to `newHeads` via websockets to get max block height
			if !config.HealthCheckConfig.UseWSForBlockHeight {
				go h.checkMaxBlockHeightByHTTP(config.ID, httpClient)
			}

			if !h.nodeIDToStatus[config.ID].getPeerCountError.isMethodNotSupportedErr() {
				go h.checkPeerCount(config.ID, httpClient)
			}

			if !h.nodeIDToStatus[config.ID].getIsSyncingError.isMethodNotSupportedErr() {
				go h.checkIsNodeSyncing(config.ID, httpClient)
			}
		}

		time.Sleep(periodicHealthCheckInterval)
	}
}

func (h *HealthCheckManager) checkMaxBlockHeightByHTTP(nodeID string, httpClient EthClient) {
	header, err := httpClient.HeaderByNumber(context.Background(), nil)

	zap.L().Debug("Running checkMaxBlockHeightByHTTP on config", zap.Any("nodeID", nodeID), zap.Any("response", header), zap.Error(err))

	h.statusMutex.Lock()
	defer h.statusMutex.Unlock()

	if err != nil {
		h.nodeIDToStatus[nodeID].getCurrentBlockNumberError = healthCheckError{err: err}
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
		h.nodeIDToStatus[nodeID].getPeerCountError = healthCheckError{err: err}
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
		h.nodeIDToStatus[nodeID].getIsSyncingError = healthCheckError{err: err}
		return
	}

	h.nodeIDToStatus[nodeID].isSyncing = syncProgress != nil
}
