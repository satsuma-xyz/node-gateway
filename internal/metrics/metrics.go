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
	metricsNamespace         = "node_gateway"
	defaultReadHeaderTimeout = 10 * time.Second

	// Metric labels
	BlockHeightCheckErrorTypeWSSubscribe = "wsSubscribe"
	BlockHeightCheckErrorTypeWSError     = "wsError"
	BlockHeightCheckErrorTypeHTTP        = "http"
)

var (
	// Overall metrics

	rpcRequestsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "server",
			Name:      "rpc_requests_total",
			Help:      "Count of total RPC requests.",
		},
		[]string{"code", "method"},
	)

	rpcRequestsDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "server",
			Name:      "rpc_request_duration_seconds",
			Help:      "Histogram of RPC request latencies.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"code", "method"},
	)

	rpcResponseSizes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "server",
			Name:      "rpc_response_size_bytes",
			Help:      "Histogram of RPC response sizes.",
			Buckets:   []float64{100, 500, 1000, 5000, 10000},
		},
		[]string{"code", "method"},
	)

	// Upstream routing metrics

	UpstreamRPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "router",
			Name:      "upstream_rpc_requests_total",
			Help:      "Count of total RPC requests forwarded to upstreams.",
		},
		[]string{"client", "endpoint_id", "url", "jsonrpc_method"},
	)

	UpstreamRPCRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "router",
			Name:      "upstream_rpc_request_errors_total",
			Help:      "Count of total errors when forwarding RPC requests to upstreams.",
		},
		[]string{"client", "endpoint_id", "url", "jsonrpc_method", "response_code", "jsonrpc_error_code"},
	)

	UpstreamRPCDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "router",
			Name:      "upstream_rpc_duration_seconds",
			Help:      "Latency of RPC requests forwarded to upstreams.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"client", "endpoint_id", "url", "jsonrpc_method", "response_code", "jsonrpc_error_code"},
	)

	// Health check metrics

	BlockHeight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "block_height",
			Help:      "Block height of upstream.",
		},
		[]string{"endpoint_id", "url"},
	)

	BlockHeightCheckRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "block_height_check_requests",
			Help:      "Total block height requests made.",
		},
		[]string{"endpoint_id", "url"},
	)

	BlockHeightCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "block_height_check_duration_seconds",
			Help:      "Latency of block height requests.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"endpoint_id", "url"},
	)

	BlockHeightCheckErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "block_height_check_errors",
			Help:      "Errors when retrieving block height of upstream.",
		},
		[]string{"endpoint_id", "url", "errorType"},
	)

	PeerCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "peer_count",
			Help:      "Block height of upstream.",
		},
		[]string{"endpoint_id", "url"},
	)

	PeerCountCheckRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "peer_count_check_requests",
			Help:      "Total peer count requests made.",
		},
		[]string{"endpoint_id", "url"},
	)

	PeerCountCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "peer_count_check_duration_seconds",
			Help:      "Latency of peer count requests.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"endpoint_id", "url"},
	)

	PeerCountCheckErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "peer_count_check_errors",
			Help:      "Errors when retrieving peer count of upstream.",
		},
		[]string{"endpoint_id", "url"},
	)

	// Use 0 or 1
	SyncStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "sync_status",
			Help:      "Sync Status of upstream.",
		},
		[]string{"endpoint_id", "url"},
	)

	SyncStatusCheckRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "sync_status_check_requests",
			Help:      "Total sync status requests made.",
		},
		[]string{"endpoint_id", "url"},
	)

	SyncStatusCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "sync_status_check_duration_seconds",
			Help:      "Latency of sync status requests.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"endpoint_id", "url"},
	)

	SyncStatusCheckErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: "healthcheck",
			Name:      "sync_status_check_errors",
			Help:      "Errors when retrieving sync status of upstream.",
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
