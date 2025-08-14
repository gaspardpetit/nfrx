package worker

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/you/llamapool/internal/logx"
)

var (
	connectedToServerGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "llamapool_worker_connected_to_server",
		Help: "Whether the worker is connected to the server (1 or 0)",
	})
	connectedToOllamaGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "llamapool_worker_connected_to_ollama",
		Help: "Whether the worker can reach its Ollama backend (1 or 0)",
	})
	currentJobsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "llamapool_worker_current_jobs",
		Help: "Number of jobs currently being processed",
	})
	maxConcurrencyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "llamapool_worker_max_concurrency",
		Help: "Maximum number of concurrent jobs",
	})
	jobsStartedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "llamapool_worker_jobs_started_total",
		Help: "Total number of jobs started",
	})
	jobsSucceededCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "llamapool_worker_jobs_succeeded_total",
		Help: "Total number of jobs that succeeded",
	})
	jobsFailedCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "llamapool_worker_jobs_failed_total",
		Help: "Total number of jobs that failed",
	})
	jobDurationHist = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "llamapool_worker_job_duration_seconds",
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
		connectedToOllamaGauge,
		currentJobsGauge,
		maxConcurrencyGauge,
		jobsStartedCounter,
		jobsSucceededCounter,
		jobsFailedCounter,
		jobDurationHist,
	)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	actual := ln.Addr().String()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logx.Log.Error().Err(err).Str("addr", actual).Msg("metrics server error")
		}
	}()
	return actual, nil
}

func setConnectedToServer(v bool) {
	if v {
		connectedToServerGauge.Set(1)
	} else {
		connectedToServerGauge.Set(0)
	}
}

func setConnectedToOllama(v bool) {
	if v {
		connectedToOllamaGauge.Set(1)
	} else {
		connectedToOllamaGauge.Set(0)
	}
}

func setCurrentJobs(n int) {
	currentJobsGauge.Set(float64(n))
}

func setMaxConcurrency(n int) {
	maxConcurrencyGauge.Set(float64(n))
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
