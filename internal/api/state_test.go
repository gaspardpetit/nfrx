package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gaspardpetit/llamapool/internal/ctrl"
)

func TestGetState(t *testing.T) {
	metricsReg := ctrl.NewMetricsRegistry("v", "sha", "date")
	metricsReg.UpsertWorker("w1", "1", "a", "d", []string{"m"})
	metricsReg.SetWorkerStatus("w1", ctrl.StatusConnected)
	metricsReg.RecordJobStart("w1")
	metricsReg.RecordJobEnd("w1", "m", 50*time.Millisecond, 5, 7, true, "")

	h := &StateHandler{Metrics: metricsReg}
	r := httptest.NewRequest(http.MethodGet, "/v1/state", nil)
	w := httptest.NewRecorder()
	h.GetState(w, r)
	var resp ctrl.StateResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Workers) != 1 || resp.Server.JobsCompletedTotal != 1 {
		t.Fatalf("bad response %+v", resp)
	}
}

func TestGetStateStream(t *testing.T) {
	metricsReg := ctrl.NewMetricsRegistry("v", "sha", "date")
	h := &StateHandler{Metrics: metricsReg}

	r := chi.NewRouter()
	r.Get("/v1/state/stream", h.GetStateStream)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/state/stream")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if line == "" {
		t.Fatalf("empty stream")
	}
}
