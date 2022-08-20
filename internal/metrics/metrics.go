package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	conf "github.com/satsuma-data/node-gateway/internal/config"
)

const (
	defaultServerPort        = 9090
	metricsNamespace         = "satsuma"
	metricsSubsystem         = "node_gateway"
	defaultReadHeaderTimeout = 10 * time.Second
)

var (
	RPCRequestsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsSubsystem,
		Name:      "rpc_requests_total",
		Help:      "Count of total RPC requests.",
	})
)

func NewMetricsServer(config conf.Config) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/", promhttp.Handler())

	port := defaultServerPort
	if config.Global.MetricsPort > 0 {
		port = config.Global.MetricsPort
	}

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}
}
