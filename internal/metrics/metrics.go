package metrics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const (
	DefaultPort                 = 9090
	MetricsNamespace            = "node_gateway"
	defaultReadHeaderTimeout    = 10 * time.Second
	systemStatsEmissionInterval = 60 * time.Second

	// Metric labels

	// HTTPInit General health check error types
	HTTPInit    = "httpInit"
	HTTPRequest = "httpReq"

	// WSSubscribe BlockHeightCheck-specific errors
	WSSubscribe = "wsSubscribe"
	WSError     = "wsError"
)

var (
	// Overall metrics

	rpcRequestsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "server",
			Name:      "rpc_requests",
			Help:      "Count of total RPC requests.",
		},
		[]string{"chain_name", "code", "method"},
	)

	rpcRequestsDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricsNamespace,
			Subsystem: "server",
			Name:      "rpc_request_duration_seconds",
			Help:      "Histogram of RPC request latencies.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"chain_name", "code", "method"},
	)

	rpcResponseSizes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricsNamespace,
			Subsystem: "server",
			Name:      "rpc_response_size_bytes",
			Help:      "Histogram of RPC response sizes.",
			Buckets:   []float64{100, 500, 1000, 5000, 10000},
		},
		[]string{"chain_name", "code", "method"},
	)

	// Upstream routing metrics

	upstreamRPCRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "router",
			Name:      "upstream_rpc_requests",
			Help:      "Count of total RPC requests forwarded to upstreams.",
		},
		// jsonrpc_method is  "batch" for batch requests
		[]string{"chain_name", "client", "upstream_id", "url", "jsonrpc_method", "cached"},
	)

	upstreamRPCRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "router",
			Name:      "upstream_rpc_request_errors",
			Help:      "Count of total errors when forwarding RPC requests to upstreams.",
		},
		// jsonrpc_method is "batch" for batch requests
		[]string{"chain_name", "client", "upstream_id", "url", "jsonrpc_method", "response_code"},
	)

	upstreamJSONRPCRequestErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "router",
			Name:      "upstream_jsonrpc_request_errors",
			Help: "Count of total errors when forwarding RPC requests to upstreams, including ones in batches. " +
				"Batches are deconstructed to single JSON RPC requests for this metric.",
		},
		// jsonrpc_method is "batch" for batch requests
		[]string{"chain_name", "client", "upstream_id", "url", "jsonrpc_method", "jsonrpc_error_code"},
	)

	upstreamRPCDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricsNamespace,
			Subsystem: "router",
			Name:      "upstream_rpc_duration_seconds",
			Help:      "Latency of RPC requests forwarded to upstreams.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		// jsonrpc_method is "batch" for batch requests
		[]string{"chain_name", "client", "upstream_id", "url", "jsonrpc_method", "response_code"},
	)

	// Health check metrics

	blockHeight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "block_height",
			Help:      "Block height of upstream.",
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	blockHeightCheckRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "block_height_check",
			Help:      "Total block height requests made.",
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	blockHeightCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "block_height_check_duration_seconds",
			Help:      "Latency of block height requests.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	blockHeightCheckErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "block_height_check_errors",
			Help:      "Errors when retrieving block height of upstream.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType"},
	)

	peerCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "peer_count",
			Help:      "Block height of upstream.",
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	peerCountCheckRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "peer_count_check_requests",
			Help:      "Total peer count requests made.",
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	peerCountCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "peer_count_check_duration_seconds",
			Help:      "Latency of peer count requests.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	peerCountCheckErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "peer_count_check_errors",
			Help:      "Errors when retrieving peer count of upstream.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType"},
	)

	// Use 0 or 1
	syncStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "sync_status",
			Help:      "Sync Status of upstream.",
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	syncStatusCheckRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "sync_status_check_requests",
			Help:      "Total sync status requests made.",
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	syncStatusCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "sync_status_check_duration_seconds",
			Help:      "Latency of sync status requests.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 20, 40},
		},
		[]string{"chain_name", "upstream_id", "url"},
	)

	syncStatusCheckErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "sync_status_check_errors",
			Help:      "Errors when retrieving sync status of upstream.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType"},
	)

	// Enhanced routing control metrics
	errorStatusCheckErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "error_check_has_errors",
			Help:      "Number of errors of upstream requests.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType", "method"},
	)

	errorStatusCheckNoErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "error_check_no_errors",
			Help:      "Number of no errors of upstream requests.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType", "method"},
	)

	errorStatusCheckErrorsIsPassing = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "error_check_is_passing",
			Help:      "Number of passing error checks.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType"},
	)

	errorStatusCheckErrorsIsFailing = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "error_check_is_failing",
			Help:      "Number of failing error checks.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType"},
	)

	latencyStatusHighLatencies = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "latency_check_high_latency",
			Help:      "Latency of upstream too high.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType", "method"},
	)

	latencyStatusOkLatencies = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "latency_check_ok_latency",
			Help:      "Latency of upstream OK.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType", "method"},
	)

	latencyStatusCheckLatencyIsPassing = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "latency_check_is_passing",
			Help:      "Number of passing latency checks.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType", "method"},
	)

	latencyStatusCheckLatencyIsFailing = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: MetricsNamespace,
			Subsystem: "healthcheck",
			Name:      "latency_check_is_failing",
			Help:      "Number of failing latency checks.",
		},
		[]string{"chain_name", "upstream_id", "url", "errorType", "method"},
	)

	// System metrics
	fileDescriptorsUsed = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: MetricsNamespace,
			Name:      "file_descriptors_used",
			Help:      "Count of Unix file descriptors used.",
		},
	)
)

type Container struct {
	RPCRequestsCounter  *prometheus.CounterVec
	RPCRequestsDuration prometheus.ObserverVec
	RPCResponseSizes    prometheus.ObserverVec

	UpstreamRPCRequestsTotal          *prometheus.CounterVec
	UpstreamRPCRequestErrorsTotal     *prometheus.CounterVec
	UpstreamJSONRPCRequestErrorsTotal *prometheus.CounterVec
	UpstreamRPCDuration               prometheus.ObserverVec

	BlockHeight              *prometheus.GaugeVec
	BlockHeightCheckRequests *prometheus.CounterVec
	BlockHeightCheckDuration prometheus.ObserverVec
	BlockHeightCheckErrors   *prometheus.CounterVec

	PeerCount              *prometheus.GaugeVec
	PeerCountCheckRequests *prometheus.CounterVec
	PeerCountCheckDuration prometheus.ObserverVec
	PeerCountCheckErrors   *prometheus.CounterVec

	SyncStatus              *prometheus.GaugeVec
	SyncStatusCheckRequests *prometheus.CounterVec
	SyncStatusCheckDuration prometheus.ObserverVec
	SyncStatusCheckErrors   *prometheus.CounterVec

	// Enhanced routing control metrics
	ErrorCheckNoErrors           *prometheus.CounterVec
	ErrorCheckErrors             *prometheus.CounterVec
	ErrorCheckErrorsIsPassing    *prometheus.CounterVec
	ErrorCheckErrorsIsFailing    *prometheus.CounterVec
	LatencyCheckOkLatencies      *prometheus.CounterVec
	LatencyCheckHighLatencies    *prometheus.CounterVec
	LatencyCheckLatencyIsPassing *prometheus.CounterVec
	LatencyCheckLatencyIsFailing *prometheus.CounterVec
}

func NewContainer(chainName string) *Container {
	result := new(Container)
	presetLabels := prometheus.Labels{
		"chain_name": chainName,
	}

	result.UpstreamRPCRequestsTotal = upstreamRPCRequestsTotal.MustCurryWith(presetLabels)
	result.UpstreamRPCRequestErrorsTotal = upstreamRPCRequestErrorsTotal.MustCurryWith(presetLabels)
	result.UpstreamJSONRPCRequestErrorsTotal = upstreamJSONRPCRequestErrorsTotal.MustCurryWith(presetLabels)
	result.UpstreamRPCDuration = upstreamRPCDuration.MustCurryWith(presetLabels)

	result.RPCRequestsCounter = rpcRequestsCounter.MustCurryWith(presetLabels)
	result.RPCRequestsDuration = rpcRequestsDuration.MustCurryWith(presetLabels)
	result.RPCResponseSizes = rpcResponseSizes.MustCurryWith(presetLabels)

	result.BlockHeight = blockHeight.MustCurryWith(presetLabels)
	result.BlockHeightCheckRequests = blockHeightCheckRequests.MustCurryWith(presetLabels)
	result.BlockHeightCheckDuration = blockHeightCheckDuration.MustCurryWith(presetLabels)
	result.BlockHeightCheckErrors = blockHeightCheckErrors.MustCurryWith(presetLabels)

	result.PeerCount = peerCount.MustCurryWith(presetLabels)
	result.PeerCountCheckRequests = peerCountCheckRequests.MustCurryWith(presetLabels)
	result.PeerCountCheckDuration = peerCountCheckDuration.MustCurryWith(presetLabels)
	result.PeerCountCheckErrors = peerCountCheckErrors.MustCurryWith(presetLabels)

	result.SyncStatus = syncStatus.MustCurryWith(presetLabels)
	result.SyncStatusCheckRequests = syncStatusCheckRequests.MustCurryWith(presetLabels)
	result.SyncStatusCheckDuration = syncStatusCheckDuration.MustCurryWith(presetLabels)
	result.SyncStatusCheckErrors = syncStatusCheckErrors.MustCurryWith(presetLabels)

	result.ErrorCheckErrors = errorStatusCheckErrors.MustCurryWith(presetLabels)
	result.ErrorCheckNoErrors = errorStatusCheckNoErrors.MustCurryWith(presetLabels)
	result.ErrorCheckErrorsIsPassing = errorStatusCheckErrorsIsPassing.MustCurryWith(presetLabels)
	result.ErrorCheckErrorsIsFailing = errorStatusCheckErrorsIsFailing.MustCurryWith(presetLabels)
	result.LatencyCheckHighLatencies = latencyStatusHighLatencies.MustCurryWith(presetLabels)
	result.LatencyCheckOkLatencies = latencyStatusOkLatencies.MustCurryWith(presetLabels)
	result.LatencyCheckLatencyIsPassing = latencyStatusCheckLatencyIsPassing.MustCurryWith(presetLabels)
	result.LatencyCheckLatencyIsFailing = latencyStatusCheckLatencyIsFailing.MustCurryWith(presetLabels)

	return result
}

func NewMetricsServer() *Server {
	mux := http.NewServeMux()
	mux.Handle("/", promhttp.Handler())

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", DefaultPort),
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	return &Server{
		server:          server,
		shutdownChannel: make(chan int),
	}
}

type Server struct {
	server          *http.Server
	shutdownChannel chan int
}

func (m *Server) Start() error {
	m.StartEmittingSystemStats()
	return m.server.ListenAndServe()
}

func (m *Server) Shutdown() error {
	select {
	case m.shutdownChannel <- 1:
		zap.L().Debug("Metrics server is stopping")
	default:
		zap.L().Debug("Metrics server has likely already shutdown.")
	}

	return m.server.Shutdown(context.Background())
}

func getNumFileDesciptors() (int, error) {
	pid := os.Getpid()
	fds, err := os.Open(fmt.Sprintf("/proc/%d/fd", pid))

	if err != nil {
		return 0, err
	}

	defer fds.Close()

	files, err := fds.Readdirnames(-1)
	if err != nil {
		return 0, err
	}

	return len(files), nil
}

func (m *Server) StartEmittingSystemStats() {
	go func() {
		for {
			select {
			case <-m.shutdownChannel:
				return
			case <-time.After(systemStatsEmissionInterval):
				numFileDescriptors, err := getNumFileDesciptors()
				zap.L().Debug("Emitting system stats.", zap.Int("numFileDescriptors", numFileDescriptors))

				if err != nil {
					zap.L().Error("Failed to get number of file descriptors.", zap.Error(err))
				} else {
					fileDescriptorsUsed.Set(float64(numFileDescriptors))
				}
			}
		}
	}()
}

func InstrumentHandler(handler http.Handler, container *Container) http.Handler {
	withRequestsCounter := promhttp.InstrumentHandlerCounter(container.RPCRequestsCounter, handler)
	withRequestsDuration := promhttp.InstrumentHandlerDuration(container.RPCRequestsDuration, withRequestsCounter)
	withResponseSizes := promhttp.InstrumentHandlerResponseSize(container.RPCResponseSizes, withRequestsDuration)

	return withResponseSizes
}
