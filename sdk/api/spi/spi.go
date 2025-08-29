package spi

import "time"

type WorkerRef interface {
    ID() string
    Name() string
    SendChan() chan<- interface{}
    AddJob(id string, ch chan interface{})
    RemoveJob(id string)
    LastHeartbeat() time.Time
    PreferredBatchSize() int
    InFlight() int
}

type ModelInfo struct {
	ID      string
	Created int64
	Owners  []string
}

type WorkerRegistry interface {
    WorkersForLabel(label string) []WorkerRef
    IncInFlight(id string)
    DecInFlight(id string)
    AggregatedModels() []ModelInfo
    AggregatedModel(id string) (ModelInfo, bool)
}

type Scheduler interface {
    PickWorker(model string) (WorkerRef, error)
}

// PartitionJob describes a request that can be split into multiple independent
// chunks and recombined. Implemented by extensions that support partitioning.
type PartitionJob interface {
    // Size returns the total number of elements in the job.
    Size() int
    // MakeChunk builds a request body for the subrange [start, start+count).
    // It may return a smaller count if fewer elements remain.
    MakeChunk(start, count int) (body []byte, actual int)
    // Append merges a completed worker response for the subrange starting at start.
    Append(resp []byte, start int) error
    // Result returns the final assembled response body.
    Result() []byte
    // Path returns the HTTP path on the worker that handles this job (e.g., "/embeddings").
    Path() string
    // DesiredChunkSize optionally specifies the ideal chunk size for a given worker.
    // Return <= 0 to defer to the worker's preferred size.
    DesiredChunkSize(w WorkerRef) int
}

type Metrics interface {
	RecordJobStart(id string)
	RecordJobEnd(id, model string, dur time.Duration, tokensIn, tokensOut, embeddings uint64, success bool, errMsg string)
	SetWorkerStatus(id string, status WorkerStatus)
	ObserveRequestDuration(workerID, model string, dur time.Duration)
	RecordWorkerProcessingTime(workerID string, dur time.Duration)
	RecordWorkerTokens(workerID, kind string, n uint64)
	RecordModelTokens(model, kind string, n uint64)
	RecordModelRequest(model string, success bool)
	RecordModelEmbeddings(model string, n uint64)
	RecordWorkerEmbeddings(workerID string, n uint64)
	RecordWorkerEmbeddingProcessingTime(workerID string, dur time.Duration)
}

type WorkerStatus string

const (
	StatusConnected WorkerStatus = "connected"
	StatusWorking   WorkerStatus = "working"
	StatusIdle      WorkerStatus = "idle"
	StatusNotReady  WorkerStatus = "not_ready"
	StatusDraining  WorkerStatus = "draining"
	StatusGone      WorkerStatus = "gone"
)

type StateElement struct {
    ID   string
    Data func() any
    // HTML is an optional function returning an HTML fragment that renders the
    // plugin's state on the state dashboard. Return empty string or leave nil
    // if no custom view is provided.
    HTML func() string
}

type StateRegistry interface {
	Add(StateElement)
}

type ServerState interface {
    IsDraining() bool
    // SetStatus updates the global server availability status
    // (e.g., "ready", "not_ready", "draining").
    SetStatus(status string)
}

type MCPClientSnapshot struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Status    string         `json:"status"`
	Inflight  int            `json:"inflight"`
	Functions map[string]int `json:"functions"`
}

type MCPSessionSnapshot struct {
	ID         string    `json:"id"`
	ClientID   string    `json:"client_id"`
	Method     string    `json:"method"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs uint64    `json:"duration_ms"`
}

type MCPState struct {
	Clients  []MCPClientSnapshot  `json:"clients"`
	Sessions []MCPSessionSnapshot `json:"sessions"`
}
