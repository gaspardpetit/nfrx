package worker

import (
    "sort"
    "sync"
    "time"
)

type WorkerStatus string

const (
    StatusConnected WorkerStatus = "connected"
    StatusWorking   WorkerStatus = "working"
    StatusIdle      WorkerStatus = "idle"
    StatusNotReady  WorkerStatus = "not_ready"
    StatusDraining  WorkerStatus = "draining"
    StatusGone      WorkerStatus = "gone"
)

type WorkerSnapshot struct {
    ID, Name, Version, BuildSHA, BuildDate string
    Status        WorkerStatus
    ConnectedAt   time.Time
    LastHeartbeat time.Time
    MaxConcurrency int `json:"max_concurrency"`
    // Keep JSON name for UI compatibility while using generic terminology
    PreferredBatchSize int `json:"embedding_batch_size"`
    ProcessedTotal, ProcessingMsTotal uint64
    AvgProcessingMs float64
    Inflight int
    FailuresTotal uint64
    QueueLen int
    LastError string
}

type ServerSnapshot struct {
    Now                time.Time `json:"now"`
    Version            string    `json:"version"`
    BuildSHA           string    `json:"build_sha,omitempty"`
    BuildDate          string    `json:"build_date,omitempty"`
    State              string    `json:"state"`
    UptimeSeconds      uint64    `json:"uptime_s"`
    JobsInflight       int       `json:"jobs_inflight_total"`
    JobsCompletedTotal uint64    `json:"jobs_completed_total"`
    JobsFailedTotal    uint64    `json:"jobs_failed_total"`
    SchedulerQueueLen  int       `json:"scheduler_queue_len"`
}

type WorkersSummary struct { Connected, Working, Idle, NotReady, Gone int }
type ModelCount struct { Name string; Workers int }

type StateResponse struct {
    Server ServerSnapshot `json:"server"`
    WorkersSummary WorkersSummary `json:"workers_summary"`
    Models []ModelCount `json:"models"`
    Workers []WorkerSnapshot `json:"workers"`
}

type MetricsRegistry struct {
    mu sync.RWMutex
    serverStart time.Time
    serverVer, serverSHA, serverDate string
    jobsInflight int
    jobsCompletedTotal, jobsFailedTotal uint64
    schedulerQueueLen int
    workers map[string]*workerMetrics
    // state string is provided by the extension if desired
    stateFunc func() string
}

type workerMetrics struct {
    id, name string
    status WorkerStatus
    connectedAt, lastHeartbeat time.Time
    version, buildSHA, buildDate string
    maxConcurrency, preferredBatchSize int
    processedTotal, processingMsTotal uint64
    inflight int
    failuresTotal uint64
    queueLen int
    lastError string
}

func NewMetricsRegistry(serverVersion, serverSHA, serverDate string, stateFn func() string) *MetricsRegistry {
    return &MetricsRegistry{serverStart: time.Now(), serverVer: serverVersion, serverSHA: serverSHA, serverDate: serverDate, workers: make(map[string]*workerMetrics), stateFunc: stateFn}
}

func (m *MetricsRegistry) UpsertWorker(id, name, version, buildSHA, buildDate string, maxConcurrency, embeddingBatchSize int, models []string) {
    m.mu.Lock(); defer m.mu.Unlock()
    w, ok := m.workers[id]
    if !ok { w = &workerMetrics{id: id, connectedAt: time.Now()}; m.workers[id] = w }
    w.name, w.version, w.buildSHA, w.buildDate = name, version, buildSHA, buildDate
    w.maxConcurrency, w.preferredBatchSize = maxConcurrency, embeddingBatchSize
    w.lastHeartbeat = time.Now()
    if w.status == "" { w.status = StatusConnected }
}

func (m *MetricsRegistry) RemoveWorker(id string) { m.mu.Lock(); delete(m.workers, id); m.mu.Unlock() }
func (m *MetricsRegistry) SetWorkerStatus(id string, status WorkerStatus) { m.mu.Lock(); if w, ok := m.workers[id]; ok { w.status = status } ; m.mu.Unlock() }
func (m *MetricsRegistry) UpdateWorker(id string, maxConcurrency, embeddingBatchSize int, models []string) {
    m.mu.Lock(); if w, ok := m.workers[id]; ok { w.maxConcurrency = maxConcurrency; w.preferredBatchSize = embeddingBatchSize } ; m.mu.Unlock()
}
func (m *MetricsRegistry) RecordHeartbeat(id string) { m.mu.Lock(); if w, ok := m.workers[id]; ok { w.lastHeartbeat = time.Now() } ; m.mu.Unlock() }

func (m *MetricsRegistry) RecordJobStart(id string) { m.mu.Lock(); if w, ok := m.workers[id]; ok { w.inflight++ } ; m.jobsInflight++; m.mu.Unlock() }
func (m *MetricsRegistry) RecordJobEnd(id, model string, duration time.Duration, tokensIn, tokensOut, embeddings uint64, success bool, errMsg string) {
    m.mu.Lock(); defer m.mu.Unlock()
    if w, ok := m.workers[id]; ok {
        if w.inflight > 0 { w.inflight-- }
        w.processedTotal++; w.processingMsTotal += uint64(duration.Milliseconds())
        if !success { w.failuresTotal++; w.lastError = errMsg }
    }
    if m.jobsInflight > 0 { m.jobsInflight-- }
    if success { m.jobsCompletedTotal++ } else { m.jobsFailedTotal++ }
}

func (m *MetricsRegistry) SetWorkerQueueLen(id string, n int) { m.mu.Lock(); if w, ok := m.workers[id]; ok { w.queueLen = n } ; m.mu.Unlock() }
func (m *MetricsRegistry) SetSchedulerQueueLen(n int) { m.mu.Lock(); m.schedulerQueueLen = n ; m.mu.Unlock() }

// AddWorkerTokens increments worker token counters by kind ("in" or "out").
func (m *MetricsRegistry) AddWorkerTokens(id, kind string, n uint64) { /* no-op in generic base */ }

func (m *MetricsRegistry) Snapshot() StateResponse {
    m.mu.RLock(); defer m.mu.RUnlock()
    resp := StateResponse{Models: []ModelCount{}, Workers: []WorkerSnapshot{}}
    state := ""
    if m.stateFunc != nil { state = m.stateFunc() }
    resp.Server = ServerSnapshot{ State: state, Now: time.Now(), Version: m.serverVer, BuildSHA: m.serverSHA, BuildDate: m.serverDate, UptimeSeconds: uint64(time.Since(m.serverStart).Seconds()), JobsInflight: m.jobsInflight, JobsCompletedTotal: m.jobsCompletedTotal, JobsFailedTotal: m.jobsFailedTotal, SchedulerQueueLen: m.schedulerQueueLen }
    workers := make([]*workerMetrics, 0, len(m.workers))
    for _, w := range m.workers { workers = append(workers, w) }
    sort.Slice(workers, func(i, j int) bool { return workers[i].connectedAt.Before(workers[j].connectedAt) })
    for _, w := range workers {
        switch w.status { case StatusConnected: resp.WorkersSummary.Connected++; case StatusWorking: resp.WorkersSummary.Working++; case StatusIdle: resp.WorkersSummary.Idle++; case StatusNotReady: resp.WorkersSummary.NotReady++ }
        avg := 0.0; if w.processedTotal > 0 { avg = float64(w.processingMsTotal) / float64(w.processedTotal) }
        snapshot := WorkerSnapshot{ ID: w.id, Name: w.name, Status: w.status, ConnectedAt: w.connectedAt, LastHeartbeat: w.lastHeartbeat, Version: w.version, BuildSHA: w.buildSHA, BuildDate: w.buildDate, MaxConcurrency: w.maxConcurrency, PreferredBatchSize: w.preferredBatchSize, ProcessedTotal: w.processedTotal, ProcessingMsTotal: w.processingMsTotal, AvgProcessingMs: avg, Inflight: w.inflight, FailuresTotal: w.failuresTotal, QueueLen: w.queueLen, LastError: w.lastError }
        resp.Workers = append(resp.Workers, snapshot)
    }
    // Leave Models empty in generic base; extensions can expose their own catalogs
    return resp
}
