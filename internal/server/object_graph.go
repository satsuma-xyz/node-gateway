package server

import (
	"net/http"
	"time"

	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/route"
	"go.uber.org/zap"
)

type objectGraph struct {
	handler           *http.ServeMux
	singleChainGraphs []singleChainObjectGraph
}

type singleChainObjectGraph struct {
	router    route.Router
	handler   *RPCHandler
	chainName string
}

func wireSingleChainDependencies(chainConfig *config.SingleChainConfig, logger *zap.Logger) singleChainObjectGraph {
	metricContainer := metrics.NewContainer(chainConfig.ChainName)
	chainMetadataStore := metadata.NewChainMetadataStore()
	ticker := time.NewTicker(checks.PeriodicHealthCheckInterval)
	healthCheckManager := checks.NewHealthCheckManager(client.NewEthClient, chainConfig.Upstreams, chainMetadataStore, ticker, metricContainer, logger)

	enabledNodeFilters := []route.NodeFilterType{route.Healthy, route.MaxHeightForGroup, route.SimpleStateOrTracePresent, route.NearGlobalMaxHeight}
	nodeFilter := route.CreateNodeFilter(enabledNodeFilters, healthCheckManager, chainMetadataStore, logger, &chainConfig.Routing)
	routingStrategy := route.FilteringRoutingStrategy{
		NodeFilter:      nodeFilter,
		BackingStrategy: route.NewPriorityRoundRobinStrategy(logger),
		Logger:          logger,
	}

	router := route.NewRouter(chainConfig.Upstreams, chainConfig.Groups, chainMetadataStore, healthCheckManager, &routingStrategy, metricContainer, logger)

	handler := &RPCHandler{
		path:   "/" + chainConfig.ChainName,
		router: router,
		logger: logger,
	}

	return singleChainObjectGraph{
		chainName: chainConfig.ChainName,
		router:    router,
		handler:   handler,
	}
}

func wireDependenciesForAllChains(
	gatewayConfig config.Config,
	rootLogger *zap.Logger,
) objectGraph {
	singleChainDependencies := make([]singleChainObjectGraph, len(gatewayConfig.Chains))

	for chainIndex := range gatewayConfig.Chains {
		currentChainConfig := &gatewayConfig.Chains[chainIndex]
		childLogger := rootLogger.With(zap.String("chainName", currentChainConfig.ChainName))

		dependencyContainer := wireSingleChainDependencies(currentChainConfig, childLogger)
		singleChainDependencies = append(singleChainDependencies, dependencyContainer)
	}

	healthCheckHandler := &HealthCheckHandler{
		singleChainDependencies: singleChainDependencies,
		logger:                  rootLogger,
	}

	mux := http.NewServeMux()
	mux.Handle("/health", healthCheckHandler)

	for _, container := range singleChainDependencies {
		mux.Handle(container.handler.path, container.handler)
		rootLogger.Info("Registered handler for chain.", zap.String("Path", container.handler.path), zap.String("chainName", container.chainName))
	}

	return objectGraph{
		singleChainGraphs: singleChainDependencies,
		handler:           mux,
	}
}
