package server

import (
	"context"
	"fmt"
	"net/http"

	conf "github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/route"
)

const (
	defaultServerPort = 8080
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
