package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gaspardpetit/llamapool/internal/ctrl"
)

func TestRelayGenerateStream(t *testing.T) {
	reg := ctrl.NewRegistry()
	worker := &ctrl.Worker{ID: "w1", Models: map[string]bool{"m": true}, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	reg.Add(worker)
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")

	go func() {
		msg := <-worker.Send
		jr := msg.(ctrl.JobRequestMessage)
		ch := worker.Jobs[jr.JobID]
		ch <- ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(`{"response":"hi","done":false}`)}
		ch <- ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(`{"done":true}`)}
	}()

	req := GenerateRequest{Model: "m", Prompt: "hi", Stream: true}
	rr := httptest.NewRecorder()
	if err := RelayGenerateStream(context.Background(), reg, metricsReg, sched, req, rr); err != nil {
		t.Fatalf("relay error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(rr.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}

func TestRelayGenerateBusy(t *testing.T) {
	reg := ctrl.NewRegistry()
	worker := &ctrl.Worker{ID: "w1", Models: map[string]bool{"m": true}, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	worker.Send <- struct{}{}
	reg.Add(worker)
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	req := GenerateRequest{Model: "m", Prompt: "hi", Stream: true}
	rr := httptest.NewRecorder()
	err := RelayGenerateStream(context.Background(), reg, metricsReg, sched, req, rr)
	if !errors.Is(err, ErrWorkerBusy) {
		t.Fatalf("expected busy error")
	}
}
