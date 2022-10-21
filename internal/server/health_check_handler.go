package server

import (
	"net/http"

	"github.com/satsuma-data/node-gateway/internal/route"
	"go.uber.org/zap"
)

type HealthCheckHandler struct {
	logger           *zap.Logger
	routerCollection route.RouterCollection
}

func (h *HealthCheckHandler) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	if h.routerCollection.IsInitialized() {
		respondRaw(h.logger, writer, []byte("OK"), http.StatusOK)
	} else {
		respondRaw(h.logger, writer, []byte("Starting up"), http.StatusServiceUnavailable)
	}
}
