package metrics

import (
    "time"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

var (
    requestTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "nfrx_request_total", Help: "Total requests accepted by extensions"},
        []string{"ext", "plugin_type", "job_type", "label"},
    )
    requestStartedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "nfrx_request_started_total", Help: "Total requests dispatched to backends"},
        []string{"ext", "plugin_type", "job_type", "label"},
    )
    requestCompletedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "nfrx_request_completed_total", Help: "Total requests completed by backends"},
        []string{"ext", "plugin_type", "job_type", "label", "success", "error_code"},
    )
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "nfrx_request_duration_seconds", Help: "End-to-end request durations"},
        []string{"ext", "plugin_type", "job_type", "label"},
    )
    requestSizeTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "nfrx_request_size_total", Help: "Total request sizes by kind"},
        []string{"ext", "plugin_type", "job_type", "label", "size_kind"},
    )
    requestInflight = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{Name: "nfrx_request_inflight", Help: "In-flight requests"},
        []string{"ext", "plugin_type", "job_type", "label"},
    )

    // Optional per-chunk metrics for partitioned workloads
    chunkCompletedTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "nfrx_request_chunk_completed_total", Help: "Completed chunks for partitioned requests"},
        []string{"ext", "plugin_type", "job_type", "label", "worker_id", "success", "error_code"},
    )
    chunkDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "nfrx_request_chunk_duration_seconds", Help: "Chunk durations"},
        []string{"ext", "plugin_type", "job_type", "label", "worker_id"},
    )
    chunkSizeTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "nfrx_request_chunk_size_total", Help: "Chunk sizes by kind"},
        []string{"ext", "plugin_type", "job_type", "label", "worker_id", "size_kind"},
    )
)

// Register registers the request-* metrics with the provided registry.
func Register(reg spi.MetricsRegistry) {
    reg.MustRegister(requestTotal, requestStartedTotal, requestCompletedTotal, requestDuration, requestSizeTotal, requestInflight, chunkCompletedTotal, chunkDuration, chunkSizeTotal)
}

func RecordRequest(ext, pluginType, jobType, label string) {
    requestTotal.WithLabelValues(ext, pluginType, jobType, label).Inc()
}
func RecordStart(ext, pluginType, jobType, label string) {
    requestStartedTotal.WithLabelValues(ext, pluginType, jobType, label).Inc()
    requestInflight.WithLabelValues(ext, pluginType, jobType, label).Inc()
}
func RecordComplete(ext, pluginType, jobType, label, errorCode string, success bool, dur time.Duration) {
    s := "false"; if success { s = "true" }
    requestCompletedTotal.WithLabelValues(ext, pluginType, jobType, label, s, errorCode).Inc()
    requestDuration.WithLabelValues(ext, pluginType, jobType, label).Observe(dur.Seconds())
    requestInflight.WithLabelValues(ext, pluginType, jobType, label).Dec()
}
func AddSize(ext, pluginType, jobType, label, sizeKind string, n uint64) {
    if n == 0 { return }
    requestSizeTotal.WithLabelValues(ext, pluginType, jobType, label, sizeKind).Add(float64(n))
}

// Chunk-level helpers
func RecordChunkComplete(ext, pluginType, jobType, label, workerID, errorCode string, success bool, dur time.Duration) {
    s := "false"; if success { s = "true" }
    chunkCompletedTotal.WithLabelValues(ext, pluginType, jobType, label, workerID, s, errorCode).Inc()
    chunkDuration.WithLabelValues(ext, pluginType, jobType, label, workerID).Observe(dur.Seconds())
}
func AddChunkSize(ext, pluginType, jobType, label, workerID, sizeKind string, n uint64) {
    if n == 0 { return }
    chunkSizeTotal.WithLabelValues(ext, pluginType, jobType, label, workerID, sizeKind).Add(float64(n))
}

