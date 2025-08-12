package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/relay"
	"github.com/you/llamapool/internal/server"
)

func TestWorkerBusy(t *testing.T) {
	reg := ctrl.NewRegistry()
	worker := &ctrl.Worker{ID: "w1", Models: map[string]bool{"m": true}, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	worker.Send <- struct{}{}
	reg.Add(worker)
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{APIKey: "testkey", WSPath: "/workers/connect", RequestTimeout: 5 * time.Second}
	handler := server.New(reg, sched, cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req := relay.GenerateRequest{Model: "m", Prompt: "hi", Stream: true}
	b, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/generate", bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer testkey")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
	var v map[string]string
	json.NewDecoder(resp.Body).Decode(&v)
	if v["error"] != "worker_busy" {
		t.Fatalf("unexpected error body: %v", v)
	}
}
