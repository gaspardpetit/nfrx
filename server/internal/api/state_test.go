package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	llmctrl "github.com/gaspardpetit/nfrx/sdk/base/worker"
	"github.com/gaspardpetit/nfrx/server/internal/jobs"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
	"github.com/gaspardpetit/nfrx/server/internal/transfer"
)

func TestGetState(t *testing.T) {
	metricsReg := llmctrl.NewMetricsRegistry("v", "sha", "date", func() string { return "" })
	metricsReg.UpsertWorker("w1", "w1", "1", "a", "d", 1, 0, []string{"m"})
	metricsReg.SetWorkerHostInfo("w1", map[string]string{
		"host_os":                  "windows",
		"host_platform":            "windows",
		"host_platform_family":     "windows",
		"host_platform_version":    "11",
		"host_kernel_version":      "10.0",
		"host_hostname":            "box1",
		"completion_agent_version": "ollama 0.9.6",
	})
	metricsReg.SetWorkerStatus("w1", llmctrl.StatusConnected)
	metricsReg.RecordHeartbeat("w1", 12.5, 43.75)
	metricsReg.AddWorkerTokens("w1", "in", 5)
	metricsReg.AddWorkerTokens("w1", "out", 7)
	metricsReg.RecordJobStart("w1")
	metricsReg.RecordJobEnd("w1", "m", 50*time.Millisecond, 5, 7, 0, true, "")

	sr := serverstate.NewRegistry()
	sr.Add(serverstate.Element{ID: "llm", Data: func() any { return metricsReg.Snapshot() }})

	h := &StateHandler{State: sr}
	r := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	w := httptest.NewRecorder()
	h.GetState(w, r)
	var env PluginsEnvelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode env: %v", err)
	}
	raw, ok := env.Plugins["llm"]
	if !ok {
		t.Fatalf("missing llm plugin state")
	}
	b, _ := json.Marshal(raw)
	var resp llmctrl.StateResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("decode llm state: %v", err)
	}
	if len(resp.Workers) != 1 || resp.Server.JobsCompletedTotal != 1 {
		t.Fatalf("bad response %+v", resp)
	}
	if resp.Workers[0].Name != "w1" {
		t.Fatalf("expected worker name")
	}
	if resp.Workers[0].Version != "1" || resp.Workers[0].HostHostname != "box1" || resp.Workers[0].CompletionAgentVersion != "ollama 0.9.6" || resp.Workers[0].HostCPUPercent != 12.5 || resp.Workers[0].HostRAMUsedPercent != 43.75 || resp.Workers[0].InputTokensTotal != 5 || resp.Workers[0].OutputTokensTotal != 7 {
		t.Fatalf("expected host telemetry %+v", resp.Workers[0])
	}
}

func TestGetStateStream(t *testing.T) {
	metricsReg := llmctrl.NewMetricsRegistry("v", "sha", "date", func() string { return "" })
	sr := serverstate.NewRegistry()
	sr.Add(serverstate.Element{ID: "llm", Data: func() any { return metricsReg.Snapshot() }})
	h := &StateHandler{State: sr}

	r := chi.NewRouter()
	r.Get("/api/state/stream", h.GetStateStream)
	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/state/stream")
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

func TestGetStateIncludesJobsSummaryAggregates(t *testing.T) {
	jobReg := jobs.NewRegistry(transfer.NewRegistry(0), 0, 0)
	sr := serverstate.NewRegistry()
	sr.Add(serverstate.Element{ID: "jobs", Data: func() any { return jobReg.StateSnapshot() }})
	h := &StateHandler{State: sr}

	r := chi.NewRouter()
	r.Post("/api/jobs", jobReg.HandleCreateJob)
	r.Post("/api/jobs/claim", jobReg.HandleClaimJob)
	r.Post("/api/jobs/{job_id}/status", jobReg.HandleStatusUpdate)
	r.Get("/api/state", h.GetState)
	srv := httptest.NewServer(r)
	defer srv.Close()

	createReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/jobs", strings.NewReader(`{"type":"asr.transcribe"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}
	defer func() {
		_ = createResp.Body.Close()
	}()
	var created struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	claimReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/jobs/claim", strings.NewReader(`{"worker_id":"worker-1","worker_group":"asr","max_wait_seconds":0}`))
	claimReq.Header.Set("Content-Type", "application/json")
	claimResp, err := http.DefaultClient.Do(claimReq)
	if err != nil {
		t.Fatalf("claim job: %v", err)
	}
	_ = claimResp.Body.Close()
	if claimResp.StatusCode != http.StatusOK {
		t.Fatalf("claim status = %d, want %d", claimResp.StatusCode, http.StatusOK)
	}

	statusReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/jobs/"+created.JobID+"/status", strings.NewReader(`{"state":"completed"}`))
	statusReq.Header.Set("Content-Type", "application/json")
	statusResp, err := http.DefaultClient.Do(statusReq)
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	_ = statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("status update code = %d, want %d", statusResp.StatusCode, http.StatusOK)
	}

	resp, err := http.Get(srv.URL + "/api/state")
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	var env PluginsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode env: %v", err)
	}
	raw, ok := env.Plugins["jobs"]
	if !ok {
		t.Fatalf("missing jobs plugin state")
	}
	b, _ := json.Marshal(raw)
	var state jobs.StateView
	if err := json.Unmarshal(b, &state); err != nil {
		t.Fatalf("decode jobs state: %v", err)
	}
	if state.Summary.CompletedJobs != 1 {
		t.Fatalf("completed_jobs = %d, want 1", state.Summary.CompletedJobs)
	}
	if state.Summary.AvgQueueWaitMS < 0 || state.Summary.AvgEndToEndMS < 0 {
		t.Fatalf("expected non-negative timing aggregates: %+v", state.Summary)
	}
	if state.Summary.LastCompletedAt == "" {
		t.Fatalf("expected last_completed_at to be set")
	}
}
