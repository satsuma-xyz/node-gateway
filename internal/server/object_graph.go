package server

import (
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
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
	globalConfig *config.GlobalConfig,
	chainConfig *config.SingleChainConfig,
	logger *zap.Logger,
	redisReader *redis.Client,
	redisWriter *redis.Client,
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

	enabledNodeFilters := []route.NodeFilterType{
		route.Healthy,
		route.MaxHeightForGroup,
		route.MethodsAllowed,
		route.NearGlobalMaxHeight,
	}
	nodeFilter := route.CreateNodeFilter(
		enabledNodeFilters,
		healthCheckManager,
		chainMetadataStore,
		logger,
		&chainConfig.Routing,
	)

	// Determine if we should always route even if no healthy upstreams are available.
	alwaysRoute := false
	if chainConfig.Routing.AlwaysRoute != nil {
		alwaysRoute = *chainConfig.Routing.AlwaysRoute
	}

	// If we should always route, use AlwaysRouteFilteringStrategy. Otherwise, use FilteringRoutingStrategy.
	backingStrategy := route.NewPriorityRoundRobinStrategy(logger)

	var routingStrategy route.RoutingStrategy

	errorFilter := route.IsErrorRateAcceptable{
		HealthCheckManager: healthCheckManager,
		MetricsContainer:   metricContainer,
	}
	latencyFilter := route.IsLatencyAcceptable{
		HealthCheckManager: healthCheckManager,
		MetricsContainer:   metricContainer,
	}

	// These should be ordered from most important to least important.
	nodeFilters := []route.NodeFilter{
		nodeFilter,
		&errorFilter,
		&latencyFilter,
	}

	if alwaysRoute {
		routingStrategy = &route.AlwaysRouteFilteringStrategy{
			NodeFilters: nodeFilters,
			RemovableFilters: []route.NodeFilterType{
				route.GetFilterTypeName(errorFilter),
				route.GetFilterTypeName(latencyFilter),
			},
			BackingStrategy: backingStrategy,
			Logger:          logger,
		}
	} else {
		routingStrategy = &route.FilteringRoutingStrategy{
			NodeFilter:      route.NewAndFilter(nodeFilters, logger),
			BackingStrategy: backingStrategy,
			Logger:          logger,
		}
	}

	rpcCache := cache.FromClients(chainConfig.Cache, redisReader, redisWriter, metricContainer)

	router := route.NewRouter(
		chainConfig.ChainName,
		chainConfig.Cache,
		chainConfig.Upstreams,
		chainConfig.Groups,
		chainMetadataStore,
		healthCheckManager,
		routingStrategy,
		metricContainer,
		logger,
		rpcCache,
	)

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
	readerAddr, writerAddr := gatewayConfig.Global.Cache.GetRedisAddresses()

	redisReader := cache.CreateRedisReaderClient(readerAddr)
	redisWriter := cache.CreateRedisWriterClient(writerAddr)

	singleChainDependencies := make([]singleChainObjectGraph, 0, len(gatewayConfig.Chains))
	routers := make([]route.Router, 0, len(gatewayConfig.Chains))

	for chainIndex := range gatewayConfig.Chains {
		currentChainConfig := &gatewayConfig.Chains[chainIndex]
		childLogger := rootLogger.With(zap.String("chainName", currentChainConfig.ChainName))

		dependencyContainer := wireSingleChainDependencies(
			&gatewayConfig.Global,
			currentChainConfig,
			childLogger,
			redisReader,
			redisWriter,
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
