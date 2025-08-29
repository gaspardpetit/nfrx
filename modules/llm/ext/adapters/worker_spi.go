package adapters

import (
	"sort"
	"sync"
	"time"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

type WorkerRef struct{ w *baseworker.Worker }

func (w WorkerRef) ID() string                            { return w.w.ID }
func (w WorkerRef) Name() string                          { return w.w.Name }
func (w WorkerRef) SendChan() chan<- interface{}          { return w.w.Send }
func (w WorkerRef) AddJob(id string, ch chan interface{}) { w.w.AddJob(id, ch) }
func (w WorkerRef) RemoveJob(id string)                   { w.w.RemoveJob(id) }
func (w WorkerRef) LastHeartbeat() time.Time              { return w.w.LastHeartbeat }
func (w WorkerRef) PreferredBatchSize() int               { return w.w.PreferredBatchSize }
func (w WorkerRef) InFlight() int                         { return w.w.InFlight }

type WorkerRegistry struct {
	r         *baseworker.Registry
	mu        sync.Mutex
	firstSeen map[string]int64
}

func (r *WorkerRegistry) WorkersForLabel(model string) []spi.WorkerRef {
	ws := r.r.WorkersForLabel(model)
	res := make([]spi.WorkerRef, 0, len(ws))
	for _, w := range ws {
		res = append(res, WorkerRef{w})
	}
	return res
}
func (r *WorkerRegistry) IncInFlight(id string) { r.r.IncInFlight(id) }
func (r *WorkerRegistry) DecInFlight(id string) { r.r.DecInFlight(id) }
func (r *WorkerRegistry) AggregatedModels() []spi.ModelInfo {
	ws := r.r.Snapshot()
	ownersMap := make(map[string][]string)
	r.mu.Lock()
	if r.firstSeen == nil {
		r.firstSeen = make(map[string]int64)
	}
	now := time.Now().Unix()
	for _, w := range ws {
		name := w.NameValue()
		for _, id := range w.LabelKeys() {
			ownersMap[id] = append(ownersMap[id], name)
			if _, ok := r.firstSeen[id]; !ok {
				r.firstSeen[id] = now
			}
		}
	}
	// build sorted result
	var res []spi.ModelInfo
	for id, owners := range ownersMap {
		sort.Strings(owners)
		res = append(res, spi.ModelInfo{ID: id, Created: r.firstSeen[id], Owners: owners})
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	r.mu.Unlock()
	return res
}
func (r *WorkerRegistry) AggregatedModel(id string) (spi.ModelInfo, bool) {
	ws := r.r.Snapshot()
	var owners []string
	r.mu.Lock()
	if r.firstSeen == nil {
		r.firstSeen = make(map[string]int64)
	}
	now := time.Now().Unix()
	for _, w := range ws {
		if w.HasLabel(id) {
			owners = append(owners, w.NameValue())
		}
	}
	if len(owners) == 0 {
		r.mu.Unlock()
		return spi.ModelInfo{}, false
	}
	if _, ok := r.firstSeen[id]; !ok {
		r.firstSeen[id] = now
	}
	sort.Strings(owners)
	m := spi.ModelInfo{ID: id, Created: r.firstSeen[id], Owners: owners}
	r.mu.Unlock()
	return m, true
}

type Scheduler struct{ s baseworker.Scheduler }

func (s Scheduler) PickWorker(model string) (spi.WorkerRef, error) {
	w, err := s.s.PickWorker(model)
	if err != nil {
		return nil, err
	}
	return WorkerRef{w}, nil
}

type Metrics struct{ m *baseworker.MetricsRegistry }

func (m Metrics) RecordJobStart(id string) { m.m.RecordJobStart(id) }
func (m Metrics) RecordJobEnd(id, model string, dur time.Duration, tokensIn, tokensOut, embeddings uint64, success bool, errMsg string) {
	m.m.RecordJobEnd(id, model, dur, tokensIn, tokensOut, embeddings, success, errMsg)
}
func (m Metrics) SetWorkerStatus(id string, status spi.WorkerStatus) {
	m.m.SetWorkerStatus(id, baseworker.WorkerStatus(status))
}

// Extension-specific metrics are emitted directly by the plugin; no-ops here.
func (m Metrics) ObserveRequestDuration(workerID, model string, dur time.Duration)       {}
func (m Metrics) RecordWorkerProcessingTime(workerID string, dur time.Duration)          {}
func (m Metrics) RecordModelTokens(model, kind string, n uint64)                         {}
func (m Metrics) RecordModelRequest(model string, success bool)                          {}
func (m Metrics) RecordModelEmbeddings(model string, n uint64)                           {}
func (m Metrics) RecordWorkerEmbeddings(workerID string, n uint64)                       {}
func (m Metrics) RecordWorkerEmbeddingProcessingTime(workerID string, dur time.Duration) {}

func NewWorkerRegistry(r *baseworker.Registry) *WorkerRegistry {
	return &WorkerRegistry{r: r, firstSeen: make(map[string]int64)}
}
func NewScheduler(s baseworker.Scheduler) Scheduler    { return Scheduler{s} }
func NewMetrics(m *baseworker.MetricsRegistry) Metrics { return Metrics{m} }
func (m Metrics) RecordWorkerTokens(workerID, kind string, n uint64) {
	m.m.AddWorkerTokens(workerID, kind, n)
}
