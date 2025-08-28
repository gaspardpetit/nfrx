package spi

import "time"

type WorkerRef interface {
	ID() string
	Name() string
	SendChan() chan<- interface{}
	AddJob(id string, ch chan interface{})
	RemoveJob(id string)
	LastHeartbeat() time.Time
	EmbeddingBatchSize() int
	InFlight() int
}

type ModelInfo struct {
	ID      string
	Created int64
	Owners  []string
}

type WorkerRegistry interface {
	WorkersForModel(model string) []WorkerRef
	WorkersForAlias(model string) []WorkerRef
	IncInFlight(id string)
	DecInFlight(id string)
	AggregatedModels() []ModelInfo
	AggregatedModel(id string) (ModelInfo, bool)
}

type Scheduler interface {
	PickWorker(model string) (WorkerRef, error)
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
}

type StateRegistry interface {
	Add(StateElement)
}

type ServerState interface {
	IsDraining() bool
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
