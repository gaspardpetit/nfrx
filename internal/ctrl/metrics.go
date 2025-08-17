package ctrl

import (
	"sort"
	"sync"
	"time"
)

// WorkerStatus represents the current state of a worker.
type WorkerStatus string

const (
	StatusConnected WorkerStatus = "connected"
	StatusWorking   WorkerStatus = "working"
	StatusIdle      WorkerStatus = "idle"
	StatusNotReady  WorkerStatus = "not_ready"
	StatusGone      WorkerStatus = "gone"
)

// PerModelStats holds per-model aggregates.
type PerModelStats struct {
	SuccessTotal   uint64 `json:"success_total"`
	ErrorTotal     uint64 `json:"error_total"`
	TokensInTotal  uint64 `json:"tokens_in_total"`
	TokensOutTotal uint64 `json:"tokens_out_total"`
}

// WorkerSnapshot represents a snapshot of worker metrics.
type WorkerSnapshot struct {
	ID                string                   `json:"id"`
	Name              string                   `json:"name"`
	Status            WorkerStatus             `json:"status"`
	ConnectedAt       time.Time                `json:"connected_at"`
	LastHeartbeat     time.Time                `json:"last_heartbeat"`
	Version           string                   `json:"version"`
	BuildSHA          string                   `json:"build_sha,omitempty"`
	BuildDate         string                   `json:"build_date,omitempty"`
	ModelsSupported   []string                 `json:"models_supported"`
	MaxConcurrency    int                      `json:"max_concurrency"`
	ProcessedTotal    uint64                   `json:"processed_total"`
	ProcessingMsTotal uint64                   `json:"processing_ms_total"`
	AvgProcessingMs   float64                  `json:"avg_processing_ms"`
	Inflight          int                      `json:"inflight"`
	FailuresTotal     uint64                   `json:"failures_total"`
	QueueLen          int                      `json:"queue_len"`
	LastError         string                   `json:"last_error,omitempty"`
	TokensInTotal     uint64                   `json:"tokens_in_total"`
	TokensOutTotal    uint64                   `json:"tokens_out_total"`
	PerModel          map[string]PerModelStats `json:"per_model"`
}

// ServerSnapshot contains server-wide aggregates.
type ServerSnapshot struct {
	Now                time.Time `json:"now"`
	Version            string    `json:"version"`
	BuildSHA           string    `json:"build_sha,omitempty"`
	BuildDate          string    `json:"build_date,omitempty"`
	UptimeSeconds      uint64    `json:"uptime_s"`
	JobsInflight       int       `json:"jobs_inflight_total"`
	JobsCompletedTotal uint64    `json:"jobs_completed_total"`
	JobsFailedTotal    uint64    `json:"jobs_failed_total"`
	SchedulerQueueLen  int       `json:"scheduler_queue_len"`
}

// WorkersSummary summarizes workers by status.
type WorkersSummary struct {
	Connected int `json:"connected"`
	Working   int `json:"working"`
	Idle      int `json:"idle"`
	NotReady  int `json:"not_ready"`
	Gone      int `json:"gone"`
}

// ModelCount tracks number of workers supporting a model.
type ModelCount struct {
	Name    string `json:"name"`
	Workers int    `json:"workers"`
}

// MCPClientSnapshot represents a connected MCP relay client.
type MCPClientSnapshot struct {
	ID        string         `json:"id"`
	Status    string         `json:"status"`
	Inflight  int            `json:"inflight"`
	Functions map[string]int `json:"functions"`
}

// MCPSessionSnapshot describes an active MCP session.
type MCPSessionSnapshot struct {
	ID         string    `json:"id"`
	ClientID   string    `json:"client_id"`
	Method     string    `json:"method"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs uint64    `json:"duration_ms"`
}

// MCPState aggregates MCP relay and session information.
type MCPState struct {
	Clients  []MCPClientSnapshot  `json:"clients"`
	Sessions []MCPSessionSnapshot `json:"sessions"`
}

// StateResponse is the top-level snapshot returned to clients.
type StateResponse struct {
	Server         ServerSnapshot   `json:"server"`
	WorkersSummary WorkersSummary   `json:"workers_summary"`
	Models         []ModelCount     `json:"models"`
	Workers        []WorkerSnapshot `json:"workers"`
	MCP            MCPState         `json:"mcp"`
}

// MetricsRegistry maintains metrics about the server and workers.
type MetricsRegistry struct {
	mu sync.RWMutex

	serverStart time.Time
	serverVer   string
	serverSHA   string
	serverDate  string

	jobsInflight       int
	jobsCompletedTotal uint64
	jobsFailedTotal    uint64
	schedulerQueueLen  int

	workers map[string]*workerMetrics
}

type workerMetrics struct {
	id              string
	name            string
	status          WorkerStatus
	connectedAt     time.Time
	lastHeartbeat   time.Time
	version         string
	buildSHA        string
	buildDate       string
	modelsSupported []string
	maxConcurrency  int

	processedTotal    uint64
	processingMsTotal uint64
	inflight          int
	failuresTotal     uint64
	queueLen          int
	lastError         string

	tokensInTotal  uint64
	tokensOutTotal uint64

	perModel map[string]*PerModelStats
}

// NewMetricsRegistry constructs a new registry.
func NewMetricsRegistry(serverVersion, serverSHA, serverDate string) *MetricsRegistry {
	return &MetricsRegistry{
		serverStart: time.Now(),
		serverVer:   serverVersion,
		serverSHA:   serverSHA,
		serverDate:  serverDate,
		workers:     make(map[string]*workerMetrics),
	}
}

// UpsertWorker registers or updates a worker.
func (m *MetricsRegistry) UpsertWorker(id, name, version, buildSHA, buildDate string, maxConcurrency int, models []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workers[id]
	if !ok {
		w = &workerMetrics{id: id, connectedAt: time.Now(), perModel: make(map[string]*PerModelStats)}
		m.workers[id] = w
	}
	w.name = name
	w.version = version
	w.buildSHA = buildSHA
	w.buildDate = buildDate
	w.modelsSupported = models
	w.maxConcurrency = maxConcurrency
	w.lastHeartbeat = time.Now()
	if w.status == "" {
		w.status = StatusConnected
	}
}

// RemoveWorker deletes a worker from the registry.
func (m *MetricsRegistry) RemoveWorker(id string) {
	m.mu.Lock()
	delete(m.workers, id)
	m.mu.Unlock()
}

// SetWorkerStatus sets the status of a worker.
func (m *MetricsRegistry) SetWorkerStatus(id string, status WorkerStatus) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.status = status
	}
	m.mu.Unlock()
}

// UpdateWorker updates a worker's models and max concurrency.
func (m *MetricsRegistry) UpdateWorker(id string, maxConcurrency int, models []string) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.maxConcurrency = maxConcurrency
		if models != nil {
			w.modelsSupported = models
		}
	}
	m.mu.Unlock()
}

// RecordHeartbeat updates the last heartbeat time for a worker.
func (m *MetricsRegistry) RecordHeartbeat(id string) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.lastHeartbeat = time.Now()
	}
	m.mu.Unlock()
}

func (m *MetricsRegistry) UpdateWorkerModels(id string, models []string) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.modelsSupported = models
	}
	m.mu.Unlock()
}

// RecordJobStart increments inflight counters.
func (m *MetricsRegistry) RecordJobStart(id string) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.inflight++
	}
	m.jobsInflight++
	m.mu.Unlock()
}

// RecordJobEnd records the end of a job.
func (m *MetricsRegistry) RecordJobEnd(id, model string, duration time.Duration, tokensIn, tokensOut uint64, success bool, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workers[id]; ok {
		if w.inflight > 0 {
			w.inflight--
		}
		w.processedTotal++
		w.processingMsTotal += uint64(duration.Milliseconds())
		w.tokensInTotal += tokensIn
		w.tokensOutTotal += tokensOut
		if w.perModel == nil {
			w.perModel = make(map[string]*PerModelStats)
		}
		pm := w.perModel[model]
		if pm == nil {
			pm = &PerModelStats{}
			w.perModel[model] = pm
		}
		pm.TokensInTotal += tokensIn
		pm.TokensOutTotal += tokensOut
		if success {
			pm.SuccessTotal++
		} else {
			pm.ErrorTotal++
			w.failuresTotal++
			w.lastError = errMsg
		}
	}
	if m.jobsInflight > 0 {
		m.jobsInflight--
	}
	if success {
		m.jobsCompletedTotal++
	} else {
		m.jobsFailedTotal++
	}
}

// RecordJobError records an error for a worker and model.
func (m *MetricsRegistry) RecordJobError(id, model, errMsg string) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.failuresTotal++
		w.lastError = errMsg
		if w.perModel == nil {
			w.perModel = make(map[string]*PerModelStats)
		}
		pm := w.perModel[model]
		if pm == nil {
			pm = &PerModelStats{}
			w.perModel[model] = pm
		}
		pm.ErrorTotal++
	}
	m.jobsFailedTotal++
	m.mu.Unlock()
}

// SetWorkerQueueLen sets a worker's internal queue length.
func (m *MetricsRegistry) SetWorkerQueueLen(id string, n int) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.queueLen = n
	}
	m.mu.Unlock()
}

// SetSchedulerQueueLen sets the scheduler queue length.
func (m *MetricsRegistry) SetSchedulerQueueLen(n int) {
	m.mu.Lock()
	m.schedulerQueueLen = n
	m.mu.Unlock()
}

// Snapshot returns a consistent snapshot of all metrics.
func (m *MetricsRegistry) Snapshot() StateResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()

	resp := StateResponse{
		Models:  []ModelCount{},
		Workers: []WorkerSnapshot{},
	}
	resp.Server = ServerSnapshot{
		Now:                time.Now(),
		Version:            m.serverVer,
		BuildSHA:           m.serverSHA,
		BuildDate:          m.serverDate,
		UptimeSeconds:      uint64(time.Since(m.serverStart).Seconds()),
		JobsInflight:       m.jobsInflight,
		JobsCompletedTotal: m.jobsCompletedTotal,
		JobsFailedTotal:    m.jobsFailedTotal,
		SchedulerQueueLen:  m.schedulerQueueLen,
	}

	modelWorkers := make(map[string]int)
	workers := make([]*workerMetrics, 0, len(m.workers))
	for _, w := range m.workers {
		workers = append(workers, w)
	}
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].connectedAt.Before(workers[j].connectedAt)
	})
	for _, w := range workers {
		switch w.status {
		case StatusConnected:
			resp.WorkersSummary.Connected++
		case StatusWorking:
			resp.WorkersSummary.Working++
		case StatusIdle:
			resp.WorkersSummary.Idle++
		case StatusNotReady:
			resp.WorkersSummary.NotReady++
		}
		for _, mname := range w.modelsSupported {
			modelWorkers[mname]++
		}
		avg := 0.0
		if w.processedTotal > 0 {
			avg = float64(w.processingMsTotal) / float64(w.processedTotal)
		}
		perModel := make(map[string]PerModelStats, len(w.perModel))
		for k, v := range w.perModel {
			perModel[k] = *v
		}
		snapshot := WorkerSnapshot{
			ID:                w.id,
			Name:              w.name,
			Status:            w.status,
			ConnectedAt:       w.connectedAt,
			LastHeartbeat:     w.lastHeartbeat,
			Version:           w.version,
			BuildSHA:          w.buildSHA,
			BuildDate:         w.buildDate,
			ModelsSupported:   append([]string(nil), w.modelsSupported...),
			MaxConcurrency:    w.maxConcurrency,
			ProcessedTotal:    w.processedTotal,
			ProcessingMsTotal: w.processingMsTotal,
			AvgProcessingMs:   avg,
			Inflight:          w.inflight,
			FailuresTotal:     w.failuresTotal,
			QueueLen:          w.queueLen,
			LastError:         w.lastError,
			TokensInTotal:     w.tokensInTotal,
			TokensOutTotal:    w.tokensOutTotal,
			PerModel:          perModel,
		}
		resp.Workers = append(resp.Workers, snapshot)
	}

	modelNames := make([]string, 0, len(modelWorkers))
	for name := range modelWorkers {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)
	for _, name := range modelNames {
		resp.Models = append(resp.Models, ModelCount{Name: name, Workers: modelWorkers[name]})
	}

	return resp
}
