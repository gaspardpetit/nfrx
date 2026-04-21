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
	ID                 string       `json:"id"`
	Name               string       `json:"name"`
	Version            string       `json:"version"`
	BuildSHA           string       `json:"build_sha,omitempty"`
	BuildDate          string       `json:"build_date,omitempty"`
	HostInfo           HostInfo     `json:"host_info,omitempty"`
	HostCPUPercent     float64      `json:"host_cpu_percent,omitempty"`
	HostRAMUsedPercent float64      `json:"host_ram_used_percent,omitempty"`
	InputTokensTotal   uint64       `json:"input_tokens_total"`
	OutputTokensTotal  uint64       `json:"output_tokens_total"`
	Status             WorkerStatus `json:"status"`
	ConnectedAt        time.Time    `json:"connected_at"`
	LastHeartbeat      time.Time    `json:"last_heartbeat"`
	MaxConcurrency     int          `json:"max_concurrency"`
	// Keep historical UI label for preferred batch size
	PreferredBatchSize int     `json:"embedding_batch_size"`
	ProcessedTotal     uint64  `json:"processed_total"`
	ProcessingMsTotal  uint64  `json:"processing_ms_total"`
	AvgProcessingMs    float64 `json:"avg_processing_ms"`
	Inflight           int     `json:"inflight"`
	FailuresTotal      uint64  `json:"failures_total"`
	QueueLen           int     `json:"queue_len"`
	LastError          string  `json:"last_error"`
}

type ServerSnapshot struct {
	Now                    time.Time `json:"now"`
	Version                string    `json:"version"`
	BuildSHA               string    `json:"build_sha,omitempty"`
	BuildDate              string    `json:"build_date,omitempty"`
	State                  string    `json:"state"`
	UptimeSeconds          uint64    `json:"uptime_s"`
	JobsInflight           int       `json:"jobs_inflight_total"`
	JobsCompletedTotal     uint64    `json:"jobs_completed_total"`
	JobsFailedTotal        uint64    `json:"jobs_failed_total"`
	SchedulerQueueLen      int       `json:"scheduler_queue_len"`
	SchedulerQueueCapacity int       `json:"scheduler_queue_capacity"`
}

type WorkersSummary struct {
	Connected int `json:"connected"`
	Working   int `json:"working"`
	Idle      int `json:"idle"`
	NotReady  int `json:"not_ready"`
	Gone      int `json:"gone"`
}
type ModelCount struct {
	Name    string `json:"name"`
	Workers int    `json:"workers"`
}

type StateResponse struct {
	Server         ServerSnapshot   `json:"server"`
	WorkersSummary WorkersSummary   `json:"workers_summary"`
	Models         []ModelCount     `json:"models"`
	Workers        []WorkerSnapshot `json:"workers"`
}

type HostInfo struct {
	Hostname       string `json:"hostname,omitempty"`
	OSName         string `json:"os_name,omitempty"`
	OSVersion      string `json:"os_version,omitempty"`
	WorkerVersion  string `json:"worker_version,omitempty"`
	BackendVersion string `json:"backend_version,omitempty"`
	BackendFamily  string `json:"backend_family,omitempty"`
}

type MetricsRegistry struct {
	mu                                  sync.RWMutex
	serverStart                         time.Time
	serverVer, serverSHA, serverDate    string
	jobsInflight                        int
	jobsCompletedTotal, jobsFailedTotal uint64
	schedulerQueueLen                   int
	schedulerQueueCapacity              int
	workers                             map[string]*workerMetrics
	// state string is provided by the extension if desired
	stateFunc func() string
}

type workerMetrics struct {
	id, name                            string
	status                              WorkerStatus
	connectedAt, lastHeartbeat          time.Time
	version, buildSHA, buildDate        string
	hostInfo                            HostInfo
	hostCPUPercent, hostRAMUsedPercent  float64
	inputTokensTotal, outputTokensTotal uint64
	maxConcurrency, preferredBatchSize  int
	processedTotal, processingMsTotal   uint64
	inflight                            int
	failuresTotal                       uint64
	queueLen                            int
	lastError                           string
}

func NewMetricsRegistry(serverVersion, serverSHA, serverDate string, stateFn func() string) *MetricsRegistry {
	return &MetricsRegistry{serverStart: time.Now(), serverVer: serverVersion, serverSHA: serverSHA, serverDate: serverDate, workers: make(map[string]*workerMetrics), stateFunc: stateFn}
}

func (m *MetricsRegistry) UpsertWorker(id, name, version, buildSHA, buildDate string, maxConcurrency, embeddingBatchSize int, models []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workers[id]
	if !ok {
		w = &workerMetrics{id: id, connectedAt: time.Now()}
		m.workers[id] = w
	}
	w.name, w.version, w.buildSHA, w.buildDate = name, version, buildSHA, buildDate
	w.hostInfo.WorkerVersion = version
	w.maxConcurrency, w.preferredBatchSize = maxConcurrency, embeddingBatchSize
	w.lastHeartbeat = time.Now()
	if w.status == "" {
		w.status = StatusConnected
	}
}

func (m *MetricsRegistry) SetWorkerHostInfo(id string, agentConfig map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workers[id]
	if !ok || agentConfig == nil {
		return
	}
	mergeHostInfo(&w.hostInfo, w.version, agentConfig)
}

func (m *MetricsRegistry) RemoveWorker(id string) { m.mu.Lock(); delete(m.workers, id); m.mu.Unlock() }
func (m *MetricsRegistry) SetWorkerStatus(id string, status WorkerStatus) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.status = status
	}
	m.mu.Unlock()
}
func (m *MetricsRegistry) UpdateWorker(id string, maxConcurrency, embeddingBatchSize int, models []string) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.maxConcurrency = maxConcurrency
		w.preferredBatchSize = embeddingBatchSize
	}
	m.mu.Unlock()
}
func (m *MetricsRegistry) RecordHeartbeat(id string, hostCPUPercent, hostRAMUsedPercent float64) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.lastHeartbeat = time.Now()
		w.hostCPUPercent = hostCPUPercent
		w.hostRAMUsedPercent = hostRAMUsedPercent
	}
	m.mu.Unlock()
}

func (m *MetricsRegistry) RecordJobStart(id string) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.inflight++
	}
	m.jobsInflight++
	m.mu.Unlock()
}
func (m *MetricsRegistry) RecordJobEnd(id, model string, duration time.Duration, tokensIn, tokensOut, embeddings uint64, success bool, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workers[id]; ok {
		if w.inflight > 0 {
			w.inflight--
		}
		w.processedTotal++
		w.processingMsTotal += uint64(duration.Milliseconds())
		if !success {
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

func (m *MetricsRegistry) SetWorkerQueueLen(id string, n int) {
	m.mu.Lock()
	if w, ok := m.workers[id]; ok {
		w.queueLen = n
	}
	m.mu.Unlock()
}
func (m *MetricsRegistry) SetSchedulerQueueLen(n int) {
	m.mu.Lock()
	m.schedulerQueueLen = n
	m.mu.Unlock()
}

// SetSchedulerQueueCapacity updates the configured capacity for the global scheduler queue.
func (m *MetricsRegistry) SetSchedulerQueueCapacity(n int) {
	m.mu.Lock()
	m.schedulerQueueCapacity = n
	m.mu.Unlock()
}

// AddWorkerTokens increments worker token counters by kind ("in" or "out").
func (m *MetricsRegistry) AddWorkerTokens(id, kind string, n uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workers[id]
	if !ok || n == 0 {
		return
	}
	switch kind {
	case "in":
		w.inputTokensTotal += n
	case "out":
		w.outputTokensTotal += n
	}
}

func (m *MetricsRegistry) Snapshot() StateResponse {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resp := StateResponse{Models: []ModelCount{}, Workers: []WorkerSnapshot{}}
	state := ""
	if m.stateFunc != nil {
		state = m.stateFunc()
	}
	resp.Server = ServerSnapshot{State: state, Now: time.Now(), Version: m.serverVer, BuildSHA: m.serverSHA, BuildDate: m.serverDate, UptimeSeconds: uint64(time.Since(m.serverStart).Seconds()), JobsInflight: m.jobsInflight, JobsCompletedTotal: m.jobsCompletedTotal, JobsFailedTotal: m.jobsFailedTotal, SchedulerQueueLen: m.schedulerQueueLen, SchedulerQueueCapacity: m.schedulerQueueCapacity}
	workers := make([]*workerMetrics, 0, len(m.workers))
	for _, w := range m.workers {
		workers = append(workers, w)
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].connectedAt.Before(workers[j].connectedAt) })
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
		avg := 0.0
		if w.processedTotal > 0 {
			avg = float64(w.processingMsTotal) / float64(w.processedTotal)
		}
		snapshot := WorkerSnapshot{
			ID:                 w.id,
			Name:               w.name,
			Status:             w.status,
			ConnectedAt:        w.connectedAt,
			LastHeartbeat:      w.lastHeartbeat,
			Version:            w.version,
			BuildSHA:           w.buildSHA,
			BuildDate:          w.buildDate,
			HostInfo:           w.hostInfo,
			HostCPUPercent:     w.hostCPUPercent,
			HostRAMUsedPercent: w.hostRAMUsedPercent,
			InputTokensTotal:   w.inputTokensTotal,
			OutputTokensTotal:  w.outputTokensTotal,
			MaxConcurrency:     w.maxConcurrency,
			PreferredBatchSize: w.preferredBatchSize,
			ProcessedTotal:     w.processedTotal,
			ProcessingMsTotal:  w.processingMsTotal,
			AvgProcessingMs:    avg,
			Inflight:           w.inflight,
			FailuresTotal:      w.failuresTotal,
			QueueLen:           w.queueLen,
			LastError:          w.lastError,
		}
		resp.Workers = append(resp.Workers, snapshot)
	}
	// Leave Models empty in generic base; extensions can expose their own catalogs
	return resp
}

func mergeHostInfo(dst *HostInfo, workerVersion string, agentConfig map[string]string) {
	if dst == nil {
		return
	}
	dst.WorkerVersion = workerVersion
	if v, ok := agentConfig["hostname"]; ok && v != "" {
		dst.Hostname = v
	}
	if v, ok := agentConfig["os_name"]; ok && v != "" {
		dst.OSName = v
	}
	if v, ok := agentConfig["os_version"]; ok && v != "" {
		dst.OSVersion = v
	}
	if v, ok := agentConfig["backend_version"]; ok && v != "" {
		dst.BackendVersion = v
	}
	if v, ok := agentConfig["backend_family"]; ok && v != "" {
		dst.BackendFamily = v
	}
}
