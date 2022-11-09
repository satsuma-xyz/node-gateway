package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/route"
)

const (
	defaultServerPort = 8080
	idleTimeout       = 2 * time.Minute
)

type RPCServer struct {
	httpServer       *http.Server
	routerCollection route.RouterCollection
}

func NewHTTPServer(config conf.Config, handler *http.ServeMux) *http.Server {
	port := defaultServerPort
	if config.Global.Port > 0 {
		port = config.Global.Port
	}

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		IdleTimeout:       idleTimeout,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	return httpServer
}

func NewRPCServer(httpServer *http.Server, routerCollection route.RouterCollection) *RPCServer {
	rpcServer := &RPCServer{
		httpServer:       httpServer,
		routerCollection: routerCollection,
	}

	return rpcServer
}

func (s *RPCServer) Start() error {
	s.routerCollection.Start()

	return s.httpServer.ListenAndServe()
}

func (s *RPCServer) Shutdown() error {
	return s.httpServer.Shutdown(context.Background())
}
