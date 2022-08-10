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

var NodeIDToStatus map[string]*NodeStatus
var statusMutex *sync.RWMutex
var getEthereumClient = NewEthClient // for unit testing

type HealthCheckConfig struct {
	nodeID       string
	httpURL      string
	websocketURL string
	// Should use websocket for `eth_subscribe` to `newHeads`. If not set, will use http URL to poll max block height
	shouldSubscribeNewHeads bool
}

func (c HealthCheckConfig) validate() bool {
	isValid := true
	if c.httpURL == "" {
		isValid = false

		zap.L().Error("httpUrl cannot be empty", zap.Any("config", c), zap.String("nodeId", c.nodeID))
	}

	if c.shouldSubscribeNewHeads && c.websocketURL == "" {
		isValid = false

		zap.L().Error("websocketUrl should be provided if shouldSubscribeNewHeads=true.", zap.Any("config", c), zap.String("nodeId", c.nodeID))
	}

	return isValid
}

func StartHealthChecks(configs []HealthCheckConfig) {
	zap.L().Info("Starting health checks.")

	NodeIDToStatus = make(map[string]*NodeStatus)

	for _, config := range configs {
		if !config.validate() {
			zap.L().Panic("Config not valid.", zap.Any("config", config), zap.String("nodeId", config.nodeID))
		}

		// Fail the `isSyncing` check until we perform the check.
		NodeIDToStatus[config.nodeID] = &NodeStatus{isSyncing: true}
	}

	statusMutex = &sync.RWMutex{}

	for _, config := range configs {
		if config.shouldSubscribeNewHeads {
			// TODO: handle case of subscribe failure, fall back to using HTTP polling
			go monitorMaxBlockHeight(config.nodeID, config.websocketURL)
		}
	}

	go runPeriodicChecks(configs)
}

func monitorMaxBlockHeight(nodeID, websocketURL string) {
	websocketClient, err := getEthereumClient(websocketURL)

	if err != nil {
		zap.L().Error("Failed to connect to node to monitor max block height.", zap.String("nodeID", nodeID), zap.Error(err))
		statusMutex.Lock()
		NodeIDToStatus[nodeID].getCurrentBlockNumberError = err
		statusMutex.Unlock()

		return
	}

	onNewHead := func(header *types.Header) {
		statusMutex.Lock()
		NodeIDToStatus[nodeID].currentBlockNumber = header.Number.Uint64()
		statusMutex.Unlock()
	}

	onError := func(failure string) {
		statusMutex.Lock()
		NodeIDToStatus[nodeID].getCurrentBlockNumberError = errors.New(failure)
		statusMutex.Unlock()
	}

	if err = subscribeNewHead(websocketClient, &newHeadHandler{onNewHead: onNewHead, onError: onError}); err != nil {
		zap.L().Error("Failed to subscribe to new head to monitor max block height.", zap.String("nodeID", nodeID), zap.Error(err))
		statusMutex.Lock()
		NodeIDToStatus[nodeID].getCurrentBlockNumberError = err
		statusMutex.Unlock()

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

func runPeriodicChecks(configs []HealthCheckConfig) {
	for {
		for _, config := range configs {
			httpClient, err := getEthereumClient(config.httpURL)
			if err != nil {
				zap.L().Error("Failed to connect to node to run periodic checks.", zap.String("nodeID", config.nodeID), zap.Error(err))
				statusMutex.Lock()
				NodeIDToStatus[config.nodeID].connectionError = err
				statusMutex.Unlock()

				continue
			}

			// Get the max block height if we're not doing `eth_subscribe` to `newHeads` via websockets
			if !config.shouldSubscribeNewHeads {
				go checkMaxBlockHeight(config.nodeID, httpClient)
			}

			go checkPeerCount(config.nodeID, httpClient)
			go checkIsNodeSyncing(config.nodeID, httpClient)
		}

		time.Sleep(PeriodicHealthCheckInterval)
	}
}

func checkMaxBlockHeight(nodeID string, httpClient EthClient) {
	header, err := httpClient.HeaderByNumber(context.Background(), nil)

	statusMutex.Lock()
	defer statusMutex.Unlock()

	if err != nil {
		NodeIDToStatus[nodeID].getCurrentBlockNumberError = err
		return
	}

	NodeIDToStatus[nodeID].currentBlockNumber = header.Number.Uint64()
}

func checkPeerCount(nodeID string, httpClient EthClient) {
	peerCount, err := httpClient.PeerCount(context.Background())

	statusMutex.Lock()
	defer statusMutex.Unlock()

	if err != nil {
		NodeIDToStatus[nodeID].getPeerCountError = err
		return
	}

	NodeIDToStatus[nodeID].peerCount = peerCount
}

func checkIsNodeSyncing(nodeID string, httpClient EthClient) {
	syncProgress, err := httpClient.SyncProgress(context.Background())

	statusMutex.Lock()
	defer statusMutex.Unlock()

	if err != nil {
		NodeIDToStatus[nodeID].getIsSyncingError = err
		return
	}

	NodeIDToStatus[nodeID].isSyncing = syncProgress != nil
}
