package api

import (
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	"time"
)

type testWorker struct {
	id     string
	name   string
	models map[string]bool
	send   chan interface{}
	jobs   map[string]chan interface{}
	hb     time.Time
	infl   int
	embBS  int
}

func newTestWorker(id string, models []string) *testWorker {
	m := map[string]bool{}
	for _, s := range models {
		m[s] = true
	}
	return &testWorker{id: id, models: m, send: make(chan interface{}, 4), jobs: map[string]chan interface{}{}, hb: time.Now()}
}

func (w *testWorker) ID() string { return w.id }
func (w *testWorker) Name() string {
	if w.name != "" {
		return w.name
	}
	return w.id
}
func (w *testWorker) SendChan() chan<- interface{}          { return w.send }
func (w *testWorker) AddJob(id string, ch chan interface{}) { w.jobs[id] = ch }
func (w *testWorker) RemoveJob(id string) {
	if ch, ok := w.jobs[id]; ok {
		delete(w.jobs, id)
		if ch != nil {
			close(ch)
		}
	}
}
func (w *testWorker) LastHeartbeat() time.Time { return w.hb }
func (w *testWorker) PreferredBatchSize() int  { return w.embBS }
func (w *testWorker) InFlight() int            { return w.infl }

type testReg struct{ w *testWorker }

func (r testReg) WorkersForLabel(model string) []spi.WorkerRef {
	if r.w.models[model] {
		return []spi.WorkerRef{r.w}
	}
	return nil
}
func (r testReg) IncInFlight(id string) { r.w.infl++ }
func (r testReg) DecInFlight(id string) {
	if r.w.infl > 0 {
		r.w.infl--
	}
}
func (r testReg) AggregatedModels() []spi.ModelInfo               { return nil }
func (r testReg) AggregatedModel(id string) (spi.ModelInfo, bool) { return spi.ModelInfo{}, false }

type testSched struct{ w spi.WorkerRef }

func (s testSched) PickWorker(model string) (spi.WorkerRef, error) { return s.w, nil }

type testMetrics struct{}

func (testMetrics) RecordJobStart(string) {}
func (testMetrics) RecordJobEnd(string, string, time.Duration, uint64, uint64, uint64, bool, string) {
}
func (testMetrics) SetWorkerStatus(string, spi.WorkerStatus)                  {}
func (testMetrics) ObserveRequestDuration(string, string, time.Duration)      {}
func (testMetrics) RecordWorkerProcessingTime(string, time.Duration)          {}
func (testMetrics) RecordWorkerTokens(string, string, uint64)                 {}
func (testMetrics) RecordModelTokens(string, string, uint64)                  {}
func (testMetrics) RecordModelRequest(string, bool)                           {}
func (testMetrics) RecordModelEmbeddings(string, uint64)                      {}
func (testMetrics) RecordWorkerEmbeddings(string, uint64)                     {}
func (testMetrics) RecordWorkerEmbeddingProcessingTime(string, time.Duration) {}

type testMultiReg struct{ ws []spi.WorkerRef }

func (r testMultiReg) WorkersForLabel(model string) []spi.WorkerRef {
	return append([]spi.WorkerRef(nil), r.ws...)
}
func (r testMultiReg) IncInFlight(id string)                           {}
func (r testMultiReg) DecInFlight(id string)                           {}
func (r testMultiReg) AggregatedModels() []spi.ModelInfo               { return nil }
func (r testMultiReg) AggregatedModel(id string) (spi.ModelInfo, bool) { return spi.ModelInfo{}, false }
