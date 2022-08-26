package metrics

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	DefaultPort              = 9090
	metricsNamespace         = "satsuma"
	metricsSubsystem         = "node_gateway"
	defaultReadHeaderTimeout = 10 * time.Second
)

var (
	rpcRequestsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "rpc_requests_total",
			Help:      "Count of total RPC requests.",
		},
		[]string{"code", "method"},
	)

	rpcRequestsDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "rpc_request_duration_seconds",
			Help:      "Histogram of RPC request latencies.",
			Buckets:   []float64{0.1, .5, 1, 5, 10, 30, 60},
		},
		[]string{"code", "method"},
	)

	rpcResponseSizes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "rpc_response_size_bytes",
			Help:      "Histogram of RPC response sizes.",
			Buckets:   []float64{100, 500, 1000, 5000, 10000},
		},
		[]string{"code", "method"},
	)

	UpstreamRPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "upstream_rpc_requests_total",
			Help:      "Count of total RPC requests forwarded to upstreams.",
		},
		[]string{"endpoint_id", "url"},
	)

	UpstreamRPCRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "upstream_rpc_request_errors_total",
			Help:      "Count of total errors when forwarding RPC requests to upstreams.",
		},
		[]string{"endpoint_id", "url"},
	)
)

func NewMetricsServer() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/", promhttp.Handler())

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", DefaultPort),
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}
}

func InstrumentHandler(handler http.Handler) http.Handler {
	withRequestsCounter := promhttp.InstrumentHandlerCounter(rpcRequestsCounter, handler)
	withRequestsDuration := promhttp.InstrumentHandlerDuration(rpcRequestsDuration, withRequestsCounter)
	withResponseSizes := promhttp.InstrumentHandlerResponseSize(rpcResponseSizes, withRequestsDuration)

	return withResponseSizes
}
