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
	singleChainGraphs []singleChainObjectGraph
	handler           *http.ServeMux
}

type singleChainObjectGraph struct {
	chainName string
	router    route.Router
	handler   *RPCHandler
}

func wireSingleChainDependencies(config *config.SingleChainConfig, logger *zap.Logger) singleChainObjectGraph {
	metricContainer := metrics.NewContainer(config.ChainName)
	chainMetadataStore := metadata.NewChainMetadataStore()
	ticker := time.NewTicker(checks.PeriodicHealthCheckInterval)
	healthCheckManager := checks.NewHealthCheckManager(client.NewEthClient, config.Upstreams, chainMetadataStore, ticker, metricContainer, logger)

	enabledNodeFilters := []route.NodeFilterType{route.Healthy, route.MaxHeightForGroup, route.SimpleStateOrTracePresent, route.NearGlobalMaxHeight}
	nodeFilter := route.CreateNodeFilter(enabledNodeFilters, healthCheckManager, chainMetadataStore, logger, &config.Routing)
	routingStrategy := route.FilteringRoutingStrategy{
		NodeFilter:      nodeFilter,
		BackingStrategy: route.NewPriorityRoundRobinStrategy(logger),
		Logger:          logger,
	}

	router := route.NewRouter(config.Upstreams, config.Groups, chainMetadataStore, healthCheckManager, &routingStrategy, metricContainer, logger)

	handler := &RPCHandler{
		path:   "/" + config.ChainName,
		router: router,
		logger: logger,
	}

	return singleChainObjectGraph{
		chainName: config.ChainName,
		router:    router,
		handler:   handler,
	}
}

func wireDependenciesForAllChains(
	config config.Config,
	rootLogger *zap.Logger,
) objectGraph {
	var singleChainDependencies []singleChainObjectGraph
	for chainIndex := range config.Chains {
		currentChainConfig := &config.Chains[chainIndex]
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
