package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "llamapool_build_info",
			Help:        "Build information",
			ConstLabels: prometheus.Labels{"component": "server"},
		},
		[]string{"date", "sha", "version"},
	)

	modelRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llamapool_model_requests_total",
			Help: "Number of model requests",
		},
		[]string{"model", "outcome"},
	)

	modelTokens = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llamapool_model_tokens_total",
			Help: "Tokens processed per model",
		},
		[]string{"kind", "model"},
	)

	workerTokens = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llamapool_worker_tokens_total",
			Help: "Tokens processed per worker",
		},
		[]string{"worker_id", "kind"},
	)

	workerProcessing = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llamapool_worker_processing_seconds_total",
			Help: "Total processing time per worker",
		},
		[]string{"worker_id"},
	)

	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llamapool_request_duration_seconds",
			Help:    "Request duration",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"worker_id", "model"},
	)
)

// Register registers all metrics with the provided registerer.
func Register(r prometheus.Registerer) {
	r.MustRegister(buildInfo, modelRequests, modelTokens, requestDuration, workerTokens, workerProcessing)
}

// SetServerBuildInfo sets the build info metric for the server.
func SetServerBuildInfo(version, sha, date string) {
	buildInfo.WithLabelValues(date, sha, version).Set(1)
}

// RecordModelRequest increments the model request counter.
func RecordModelRequest(model string, success bool) {
	outcome := "success"
	if !success {
		outcome = "error"
	}
	modelRequests.WithLabelValues(model, outcome).Inc()
}

// RecordModelTokens increments token counters for a model.
func RecordModelTokens(model, kind string, n uint64) {
	modelTokens.WithLabelValues(kind, model).Add(float64(n))
}

// ObserveRequestDuration records the duration of a request.
func ObserveRequestDuration(workerID, model string, d time.Duration) {
	requestDuration.WithLabelValues(workerID, model).Observe(d.Seconds())
}

// RecordWorkerTokens increments token counters for a worker.
func RecordWorkerTokens(workerID, kind string, n uint64) {
	workerTokens.WithLabelValues(workerID, kind).Add(float64(n))
}

// RecordWorkerProcessingTime records processing time for a worker.
func RecordWorkerProcessingTime(workerID string, d time.Duration) {
	workerProcessing.WithLabelValues(workerID).Add(d.Seconds())
}
