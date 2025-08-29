package adapters

import (
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
func (w WorkerRef) EmbeddingBatchSize() int               { return w.w.EmbeddingBatchSize }
func (w WorkerRef) InFlight() int                         { return w.w.InFlight }

type WorkerRegistry struct{ r *baseworker.Registry }

func (r WorkerRegistry) WorkersForModel(model string) []spi.WorkerRef {
    ws := r.r.WorkersForModel(model)
    res := make([]spi.WorkerRef, 0, len(ws))
    for _, w := range ws { res = append(res, WorkerRef{w}) }
    return res
}
func (r WorkerRegistry) WorkersForAlias(model string) []spi.WorkerRef {
    ws := r.r.WorkersForAlias(model)
    res := make([]spi.WorkerRef, 0, len(ws))
    for _, w := range ws { res = append(res, WorkerRef{w}) }
    return res
}
func (r WorkerRegistry) IncInFlight(id string) { r.r.IncInFlight(id) }
func (r WorkerRegistry) DecInFlight(id string) { r.r.DecInFlight(id) }
func (r WorkerRegistry) AggregatedModels() []spi.ModelInfo {
    ms := r.r.AggregatedModels()
    res := make([]spi.ModelInfo, 0, len(ms))
    for _, m := range ms { res = append(res, spi.ModelInfo{ID: m.ID, Created: m.Created, Owners: m.Owners}) }
    return res
}
func (r WorkerRegistry) AggregatedModel(id string) (spi.ModelInfo, bool) {
    m, ok := r.r.AggregatedModel(id); if !ok { return spi.ModelInfo{}, false }
    return spi.ModelInfo{ID: m.ID, Created: m.Created, Owners: m.Owners}, true
}

type Scheduler struct{ s baseworker.Scheduler }

func (s Scheduler) PickWorker(model string) (spi.WorkerRef, error) {
    w, err := s.s.PickWorker(model)
    if err != nil { return nil, err }
    return WorkerRef{w}, nil
}

type Metrics struct{ m *baseworker.MetricsRegistry }

func (m Metrics) RecordJobStart(id string) { m.m.RecordJobStart(id) }
func (m Metrics) RecordJobEnd(id, model string, dur time.Duration, tokensIn, tokensOut, embeddings uint64, success bool, errMsg string) {
    m.m.RecordJobEnd(id, model, dur, tokensIn, tokensOut, embeddings, success, errMsg)
}
func (m Metrics) SetWorkerStatus(id string, status spi.WorkerStatus) { m.m.SetWorkerStatus(id, baseworker.WorkerStatus(status)) }

// Extension-specific metrics are emitted directly by the plugin; no-ops here.
func (m Metrics) ObserveRequestDuration(workerID, model string, dur time.Duration)               {}
func (m Metrics) RecordWorkerProcessingTime(workerID string, dur time.Duration)                   {}
func (m Metrics) RecordModelTokens(model, kind string, n uint64)                                  {}
func (m Metrics) RecordModelRequest(model string, success bool)                                   {}
func (m Metrics) RecordModelEmbeddings(model string, n uint64)                                    {}
func (m Metrics) RecordWorkerEmbeddings(workerID string, n uint64)                                {}
func (m Metrics) RecordWorkerEmbeddingProcessingTime(workerID string, dur time.Duration)          {}

func NewWorkerRegistry(r *baseworker.Registry) WorkerRegistry { return WorkerRegistry{r} }
func NewScheduler(s baseworker.Scheduler) Scheduler           { return Scheduler{s} }
func NewMetrics(m *baseworker.MetricsRegistry) Metrics        { return Metrics{m} }
func (m Metrics) RecordWorkerTokens(workerID, kind string, n uint64) { m.m.AddWorkerTokens(workerID, kind, n) }
