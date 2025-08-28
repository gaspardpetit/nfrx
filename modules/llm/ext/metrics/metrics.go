package metrics

import (
    "time"

    "github.com/prometheus/client_golang/prometheus"

    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

var (
    // Model/API level metrics
    modelRequests = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nfrx_llm_model_requests_total",
            Help: "Number of LLM model requests",
        },
        []string{"model", "outcome"},
    )

    modelTokens = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nfrx_llm_model_tokens_total",
            Help: "Tokens processed per model",
        },
        []string{"kind", "model"},
    )

    // Worker-level metrics
    workerTokens = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nfrx_llm_worker_tokens_total",
            Help: "Tokens processed per worker",
        },
        []string{"worker_id", "kind"},
    )

    workerProcessing = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nfrx_llm_worker_processing_seconds_total",
            Help: "Total processing time per worker",
        },
        []string{"worker_id"},
    )

    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "nfrx_llm_request_duration_seconds",
            Help:    "LLM request duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"worker_id", "model"},
    )

    // Embeddings-specific metrics
    modelEmbeddings = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nfrx_llm_model_embeddings_total",
            Help: "Embeddings processed per model",
        },
        []string{"model"},
    )

    workerEmbeddings = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nfrx_llm_worker_embeddings_total",
            Help: "Embeddings processed per worker",
        },
        []string{"worker_id"},
    )

    workerEmbeddingProcessing = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "nfrx_llm_worker_embedding_processing_seconds_total",
            Help: "Total embedding processing time per worker",
        },
        []string{"worker_id"},
    )
)

// Register registers all LLM extension collectors.
func Register(r spi.MetricsRegistry) {
    r.MustRegister(
        modelRequests,
        modelTokens,
        requestDuration,
        workerTokens,
        workerProcessing,
        modelEmbeddings,
        workerEmbeddings,
        workerEmbeddingProcessing,
    )
}

func RecordModelRequest(model string, success bool) {
    outcome := "success"
    if !success { outcome = "error" }
    modelRequests.WithLabelValues(model, outcome).Inc()
}

func RecordModelTokens(model, kind string, n uint64) {
    modelTokens.WithLabelValues(kind, model).Add(float64(n))
}

func ObserveRequestDuration(workerID, model string, d time.Duration) {
    requestDuration.WithLabelValues(workerID, model).Observe(d.Seconds())
}

func RecordWorkerTokens(workerID, kind string, n uint64) {
    workerTokens.WithLabelValues(workerID, kind).Add(float64(n))
}

func RecordWorkerProcessingTime(workerID string, d time.Duration) {
    workerProcessing.WithLabelValues(workerID).Add(d.Seconds())
}

func RecordModelEmbeddings(model string, n uint64) {
    modelEmbeddings.WithLabelValues(model).Add(float64(n))
}

func RecordWorkerEmbeddings(workerID string, n uint64) {
    workerEmbeddings.WithLabelValues(workerID).Add(float64(n))
}

func RecordWorkerEmbeddingProcessingTime(workerID string, d time.Duration) {
    workerEmbeddingProcessing.WithLabelValues(workerID).Add(d.Seconds())
}

