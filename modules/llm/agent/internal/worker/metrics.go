package worker

import (
	"context"
	"time"

	"github.com/gaspardpetit/nfrx/core/logx"
	"github.com/gaspardpetit/nfrx/sdk/base/agent"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	connectedToServerGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nfrx_worker_connected_to_server",
		Help: "Whether the worker is connected to the server (1 or 0)",
	})
	connectedToBackendGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nfrx_worker_connected_to_backend",
		Help: "Whether the worker can reach its completion backend (1 or 0)",
	})
	currentJobsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nfrx_worker_current_jobs",
		Help: "Number of jobs currently being processed",
	})
	maxConcurrencyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nfrx_worker_max_concurrency",
		Help: "Maximum number of concurrent jobs",
	})
	embeddingBatchSizeGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "nfrx_worker_embedding_batch_size",
		Help: "Ideal embedding batch size",
	})
	jobsStartedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nfrx_worker_jobs_started_total",
		Help: "Total number of jobs started",
	})
	jobsSucceededCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nfrx_worker_jobs_succeeded_total",
		Help: "Total number of jobs that succeeded",
	})
	jobsFailedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "nfrx_worker_jobs_failed_total",
		Help: "Total number of jobs that failed",
	})
	jobDurationHist = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "nfrx_worker_job_duration_seconds",
		Help:    "Duration of jobs in seconds",
		Buckets: prometheus.DefBuckets,
	})
)

// StartMetricsServer starts an HTTP server exposing Prometheus metrics on /metrics.
// It returns the address it is listening on.
func StartMetricsServer(ctx context.Context, addr string) (string, error) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		connectedToServerGauge,
		connectedToBackendGauge,
		currentJobsGauge,
		maxConcurrencyGauge,
		embeddingBatchSizeGauge,
		jobsStartedCounter,
		jobsSucceededCounter,
		jobsFailedCounter,
		jobDurationHist,
	)
	addrOut, err := agent.StartMetricsServer(ctx, addr, reg)
	if err != nil {
		return "", err
	}
	logx.Log.Info().Str("addr", addrOut).Msg("metrics server started")
	return addrOut, nil
}

func setConnectedToServer(v bool) {
	if v {
		connectedToServerGauge.Set(1)
	} else {
		connectedToServerGauge.Set(0)
	}
}

func setConnectedToBackend(v bool) {
	if v {
		connectedToBackendGauge.Set(1)
	} else {
		connectedToBackendGauge.Set(0)
	}
}

func setCurrentJobs(n int) {
	currentJobsGauge.Set(float64(n))
}

func setMaxConcurrency(n int) {
	maxConcurrencyGauge.Set(float64(n))
}

func setEmbeddingBatchSize(n int) {
	embeddingBatchSizeGauge.Set(float64(n))
}

// JobStarted increments the started jobs counter.
func JobStarted() {
	jobsStartedCounter.Inc()
}

// JobCompleted records the job duration and success/failure.
func JobCompleted(success bool, d time.Duration) {
	if success {
		jobsSucceededCounter.Inc()
	} else {
		jobsFailedCounter.Inc()
	}
	jobDurationHist.Observe(d.Seconds())
}
