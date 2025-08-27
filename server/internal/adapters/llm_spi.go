package adapters

import (
	"time"

	"github.com/gaspardpetit/nfrx/modules/common/spi"
	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/server/internal/metrics"
)

type WorkerRef struct {
	w *ctrlsrv.Worker
}

func (w WorkerRef) ID() string                            { return w.w.ID }
func (w WorkerRef) Name() string                          { return w.w.Name }
func (w WorkerRef) SendChan() chan<- interface{}          { return w.w.Send }
func (w WorkerRef) AddJob(id string, ch chan interface{}) { w.w.AddJob(id, ch) }
func (w WorkerRef) RemoveJob(id string)                   { w.w.RemoveJob(id) }
func (w WorkerRef) LastHeartbeat() time.Time              { return w.w.LastHeartbeat }
func (w WorkerRef) EmbeddingBatchSize() int               { return w.w.EmbeddingBatchSize }
func (w WorkerRef) InFlight() int                         { return w.w.InFlight }

type WorkerRegistry struct {
	r *ctrlsrv.Registry
}

func (r WorkerRegistry) WorkersForModel(model string) []spi.WorkerRef {
	ws := r.r.WorkersForModel(model)
	res := make([]spi.WorkerRef, 0, len(ws))
	for _, w := range ws {
		res = append(res, WorkerRef{w})
	}
	return res
}

func (r WorkerRegistry) WorkersForAlias(model string) []spi.WorkerRef {
	ws := r.r.WorkersForAlias(model)
	res := make([]spi.WorkerRef, 0, len(ws))
	for _, w := range ws {
		res = append(res, WorkerRef{w})
	}
	return res
}

func (r WorkerRegistry) IncInFlight(id string) { r.r.IncInFlight(id) }
func (r WorkerRegistry) DecInFlight(id string) { r.r.DecInFlight(id) }
func (r WorkerRegistry) AggregatedModels() []spi.ModelInfo {
	ms := r.r.AggregatedModels()
	res := make([]spi.ModelInfo, 0, len(ms))
	for _, m := range ms {
		res = append(res, spi.ModelInfo{ID: m.ID, Created: m.Created, Owners: m.Owners})
	}
	return res
}

func (r WorkerRegistry) AggregatedModel(id string) (spi.ModelInfo, bool) {
	m, ok := r.r.AggregatedModel(id)
	if !ok {
		return spi.ModelInfo{}, false
	}
	return spi.ModelInfo{ID: m.ID, Created: m.Created, Owners: m.Owners}, true
}

type Scheduler struct {
	s ctrlsrv.Scheduler
}

func (s Scheduler) PickWorker(model string) (spi.WorkerRef, error) {
	w, err := s.s.PickWorker(model)
	if err != nil {
		return nil, err
	}
	return WorkerRef{w}, nil
}

type Metrics struct {
	m *ctrlsrv.MetricsRegistry
}

func (m Metrics) RecordJobStart(id string) { m.m.RecordJobStart(id) }
func (m Metrics) RecordJobEnd(id, model string, dur time.Duration, tokensIn, tokensOut, embeddings uint64, success bool, errMsg string) {
	m.m.RecordJobEnd(id, model, dur, tokensIn, tokensOut, embeddings, success, errMsg)
}
func (m Metrics) SetWorkerStatus(id string, status spi.WorkerStatus) {
	m.m.SetWorkerStatus(id, ctrlsrv.WorkerStatus(status))
}
func (m Metrics) ObserveRequestDuration(workerID, model string, dur time.Duration) {
	metrics.ObserveRequestDuration(workerID, model, dur)
}
func (m Metrics) RecordWorkerProcessingTime(workerID string, dur time.Duration) {
	metrics.RecordWorkerProcessingTime(workerID, dur)
}
func (m Metrics) RecordWorkerTokens(workerID, kind string, n uint64) {
	metrics.RecordWorkerTokens(workerID, kind, n)
}
func (m Metrics) RecordModelTokens(model, kind string, n uint64) {
	metrics.RecordModelTokens(model, kind, n)
}
func (m Metrics) RecordModelRequest(model string, success bool) {
	metrics.RecordModelRequest(model, success)
}
func (m Metrics) RecordModelEmbeddings(model string, n uint64) {
	metrics.RecordModelEmbeddings(model, n)
}
func (m Metrics) RecordWorkerEmbeddings(workerID string, n uint64) {
	metrics.RecordWorkerEmbeddings(workerID, n)
}
func (m Metrics) RecordWorkerEmbeddingProcessingTime(workerID string, dur time.Duration) {
	metrics.RecordWorkerEmbeddingProcessingTime(workerID, dur)
}

func NewWorkerRegistry(r *ctrlsrv.Registry) WorkerRegistry { return WorkerRegistry{r} }
func NewScheduler(s ctrlsrv.Scheduler) Scheduler           { return Scheduler{s} }
func NewMetrics(m *ctrlsrv.MetricsRegistry) Metrics        { return Metrics{m} }
func NewWorkerRef(w *ctrlsrv.Worker) WorkerRef             { return WorkerRef{w} }
