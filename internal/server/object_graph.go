package server

import (
	"net/http"
	"time"

	"github.com/satsuma-data/node-gateway/internal/cache"
	"github.com/satsuma-data/node-gateway/internal/checks"
	"github.com/satsuma-data/node-gateway/internal/client"
	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/satsuma-data/node-gateway/internal/metrics"
	"github.com/satsuma-data/node-gateway/internal/route"
	"go.uber.org/zap"
)

type ObjectGraph struct {
	Handler          *http.ServeMux
	RPCServer        *RPCServer
	RouterCollection route.RouterCollection
}

type singleChainObjectGraph struct {
	router    route.Router
	handler   http.Handler
	path      string
	chainName string
}

func wireSingleChainDependencies(
	globalConfig config.GlobalConfig, //nolint:gocritic // Legacy
	chainConfig *config.SingleChainConfig,
	logger *zap.Logger,
	rpcCache *cache.RPCCache,
) singleChainObjectGraph {
	metricContainer := metrics.NewContainer(chainConfig.ChainName)
	chainMetadataStore := metadata.NewChainMetadataStore()
	ticker := time.NewTicker(checks.PeriodicHealthCheckInterval)
	healthCheckManager := checks.NewHealthCheckManager(
		client.NewEthClient,
		chainConfig.Upstreams,
		chainConfig.Routing,
		globalConfig.Routing,
		chainMetadataStore,
		ticker,
		metricContainer,
		logger,
	)

	// TODO(polsar): Here, the HealthCheckManager is wired into the primary FilteringRoutingStrategy.
	// We may need to wire it into the secondary PriorityRoundRobinStrategy as well.
	enabledNodeFilters := []route.NodeFilterType{route.Healthy, route.MaxHeightForGroup, route.MethodsAllowed, route.NearGlobalMaxHeight}
	nodeFilter := route.CreateNodeFilter(enabledNodeFilters, healthCheckManager, chainMetadataStore, logger, &chainConfig.Routing)
	routingStrategy := route.FilteringRoutingStrategy{
		NodeFilter:      nodeFilter,
		BackingStrategy: route.NewPriorityRoundRobinStrategy(logger),
		Logger:          logger,
	}

	router := route.NewRouter(chainConfig.ChainName, chainConfig.Cache, chainConfig.Upstreams, chainConfig.Groups, chainMetadataStore, healthCheckManager, &routingStrategy, metricContainer, logger, rpcCache)

	path := "/" + chainConfig.ChainName
	handler := &RPCHandler{
		path:   path,
		router: router,
		logger: logger,
	}
	handlerWithMetrics := metrics.InstrumentHandler(handler, metricContainer)

	return singleChainObjectGraph{
		chainName: chainConfig.ChainName,
		router:    router,
		handler:   handlerWithMetrics,
		path:      path,
	}
}

func WireDependenciesForAllChains(
	gatewayConfig config.Config, //nolint:gocritic // Legacy
	rootLogger *zap.Logger,
) ObjectGraph {
	rpcCache := cache.NewRPCCache(gatewayConfig.Global.Cache.Redis)

	singleChainDependencies := make([]singleChainObjectGraph, 0, len(gatewayConfig.Chains))
	routers := make([]route.Router, 0, len(gatewayConfig.Chains))

	for chainIndex := range gatewayConfig.Chains {
		currentChainConfig := &gatewayConfig.Chains[chainIndex]
		childLogger := rootLogger.With(zap.String("chainName", currentChainConfig.ChainName))

		dependencyContainer := wireSingleChainDependencies(
			gatewayConfig.Global,
			currentChainConfig,
			childLogger,
			rpcCache,
		)

		singleChainDependencies = append(singleChainDependencies, dependencyContainer)
		routers = append(routers, dependencyContainer.router)
	}

	routerCollection := route.RouterCollection{Routers: routers}

	healthCheckHandler := &HealthCheckHandler{
		routerCollection: routerCollection,
		logger:           rootLogger,
	}

	mux := newServeMux(healthCheckHandler, singleChainDependencies, rootLogger)

	httpServer := NewHTTPServer(gatewayConfig, mux)
	rpcServer := NewRPCServer(httpServer, routerCollection)

	return ObjectGraph{
		RouterCollection: routerCollection,
		Handler:          mux,
		RPCServer:        rpcServer,
	}
}

func newServeMux(
	healthCheckHandler *HealthCheckHandler,
	singleChainDependencies []singleChainObjectGraph,
	rootLogger *zap.Logger,
) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/health", healthCheckHandler)

	for _, container := range singleChainDependencies {
		mux.Handle(container.path, container.handler)
		rootLogger.Info("Registered handler for chain.", zap.String("Path", container.path), zap.String("chainName", container.chainName))
	}

	return mux
}
