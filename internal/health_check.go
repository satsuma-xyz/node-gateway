package internal

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

var PeriodicHealthCheckInterval = 1 * time.Second

type NodeStatus struct {
	getCurrentBlockNumberError error
	getPeerCountError          error
	getIsSyncingError          error
	currentBlockNumber         uint64
	peerCount                  uint64
	isSyncing                  bool
}

var NodeIDToStatus map[string]*NodeStatus
var statusMutex *sync.RWMutex

// If there's no websocket url, we use http URL to poll max block height
// instead of using the websocket URL to subscribe to new heads.
type HealthCheckConfig struct {
	// Used to `eth_subscribe` to `newHeads`.
	websocketURL string
	httpURL      string
}

func StartHealthChecks(nodeIDToConfig map[string]HealthCheckConfig) {
	zap.L().Info("Starting health checks.")

	NodeIDToStatus = make(map[string]*NodeStatus)
	for nodeID := range nodeIDToConfig {
		// Fail the `isSyncing` check until we perform the check.
		NodeIDToStatus[nodeID] = &NodeStatus{isSyncing: true}
	}

	statusMutex = &sync.RWMutex{}

	for nodeID, config := range nodeIDToConfig {
		if config.websocketURL != "" {
			go monitorMaxBlockHeight(nodeID, config.websocketURL)
		}
	}

	go runPeriodicChecks(nodeIDToConfig)
}

func monitorMaxBlockHeight(nodeID, websocketURL string) {
	websocketClient, err := ethclient.Dial(websocketURL)

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

	err = subscribeNewHead(websocketClient, &newHeadHandler{onNewHead: onNewHead, onError: onError})
	if err != nil {
		zap.L().Error("Failed to subscribe to new head to monitor max block height.", zap.String("nodeID", nodeID), zap.Error(err))
		statusMutex.Lock()
		NodeIDToStatus[nodeID].getCurrentBlockNumberError = err
		statusMutex.Unlock()

		return
	}
}

type newHeadHandler struct {
	onNewHead func(header *types.Header)
	onError   func(failure string)
}

func subscribeNewHead(websocketClient *ethclient.Client, handler *newHeadHandler) error {
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

func runPeriodicChecks(nodeIDToConfig map[string]HealthCheckConfig) {
	nodeIDToHTTPClient := make(map[string]*ethclient.Client)

	for nodeID, config := range nodeIDToConfig {
		client, err := ethclient.Dial(config.httpURL)
		if err != nil {
			zap.L().Error("Failed to connect to node to run periodic checks.", zap.String("nodeID", nodeID), zap.Error(err))
			continue
		}

		nodeIDToHTTPClient[nodeID] = client
	}

	for {
		for nodeID, httpClient := range nodeIDToHTTPClient {
			// Only get the max block height if there is no websocket URL.
			if nodeIDToConfig[nodeID].websocketURL == "" {
				go checkMaxBlockHeight(nodeID, httpClient)
			}

			go checkPeerCount(nodeID, httpClient)
			go checkIsNodeSyncing(nodeID, httpClient)
		}

		time.Sleep(PeriodicHealthCheckInterval)
	}
}

func checkMaxBlockHeight(nodeID string, httpClient *ethclient.Client) {
	header, err := httpClient.HeaderByNumber(context.Background(), nil)

	statusMutex.Lock()
	defer statusMutex.Unlock()

	if err != nil {
		NodeIDToStatus[nodeID].getCurrentBlockNumberError = err
		return
	}

	NodeIDToStatus[nodeID].currentBlockNumber = header.Number.Uint64()
}

func checkPeerCount(nodeID string, httpClient *ethclient.Client) {
	peerCount, err := httpClient.PeerCount(context.Background())

	statusMutex.Lock()
	defer statusMutex.Unlock()

	if err != nil {
		NodeIDToStatus[nodeID].getPeerCountError = err
		return
	}

	NodeIDToStatus[nodeID].peerCount = peerCount
}

func checkIsNodeSyncing(nodeID string, httpClient *ethclient.Client) {
	syncProgress, err := httpClient.SyncProgress(context.Background())

	statusMutex.Lock()
	defer statusMutex.Unlock()

	if err != nil {
		NodeIDToStatus[nodeID].getIsSyncingError = err
		return
	}

	NodeIDToStatus[nodeID].isSyncing = syncProgress != nil
}
