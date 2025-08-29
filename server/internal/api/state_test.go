package api

import (
    "bufio"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/go-chi/chi/v5"

    llmctrl "github.com/gaspardpetit/nfrx/sdk/base/worker"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

func TestGetState(t *testing.T) {
    metricsReg := llmctrl.NewMetricsRegistry("v", "sha", "date", func() string { return "" })
    metricsReg.UpsertWorker("w1", "w1", "1", "a", "d", 1, 0, []string{"m"})
    metricsReg.SetWorkerStatus("w1", llmctrl.StatusConnected)
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
