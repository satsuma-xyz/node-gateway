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

type SingleChainDependencyContainer struct {
	ChainName string
	Router    route.Router
	handler   *RPCHandler
}

func wireDependenciesForAllChains(
	config config.Config,
	rootLogger *zap.Logger,
) DependencyContainer {
	var singleChainDependencies []SingleChainDependencyContainer
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
	}

	return DependencyContainer{
		singleChainDependencies: singleChainDependencies,
		handler:                 mux,
	}
}

type DependencyContainer struct {
	singleChainDependencies []SingleChainDependencyContainer
	handler                 *http.ServeMux
}

func wireSingleChainDependencies(config *config.SingleChainConfig, logger *zap.Logger) SingleChainDependencyContainer {
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

	return SingleChainDependencyContainer{
		ChainName: config.ChainName,
		Router:    router,
		handler:   handler,
	}
}
