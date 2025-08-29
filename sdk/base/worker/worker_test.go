package worker

import (
    "context"
    "encoding/json"
    "net/http/httptest"
    "strings"
    "sync"
    "testing"
    "time"

    "github.com/coder/websocket"
    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
)

func TestScoreSchedulerLeastBusy(t *testing.T) {
    reg := NewRegistry()
    w1 := &Worker{ID: "w1", Models: map[string]bool{"m": true}, InFlight: 1, MaxConcurrency: 2}
    w2 := &Worker{ID: "w2", Models: map[string]bool{"m": true}, InFlight: 0, MaxConcurrency: 1}
    reg.Add(w1)
    reg.Add(w2)
    sched := NewScoreScheduler(reg, DefaultExactMatchScorer{})
    w, err := sched.PickWorker("m")
    if err != nil { t.Fatalf("unexpected err: %v", err) }
    if w.ID != "w2" { t.Fatalf("expected w2, got %s", w.ID) }
}

func TestLLMScorerAliasFallback(t *testing.T) {
    reg := NewRegistry()
    alias := &Worker{ID: "alias", Models: map[string]bool{"llama2:7b-fp16": true}, MaxConcurrency: 1}
    reg.Add(alias)
    // emulate llm scorer behavior
    type aScorer struct{}
    func (aScorer) Score(task string, w *Worker) float64 {
        if w.Models[task] { return 1.0 }
        if ak, ok := ctrl.AliasKey(task); ok {
            for m := range w.Models { if mk, ok2 := ctrl.AliasKey(m); ok2 && mk == ak { return 0.5 } }
        }
        return 0.0
    }
    sched := NewScoreScheduler(reg, aScorer{})
    w, err := sched.PickWorker("llama2:7b-q4_0")
    if err != nil { t.Fatalf("unexpected err: %v", err) }
    if w.ID != "alias" { t.Fatalf("expected alias, got %s", w.ID) }
}

func TestMinScoreThreshold(t *testing.T) {
    reg := NewRegistry()
    // Two capacity-available workers with no matching models (score 0)
    w1 := &Worker{ID: "w1", Models: map[string]bool{}, InFlight: 1, MaxConcurrency: 2}
    w2 := &Worker{ID: "w2", Models: map[string]bool{}, InFlight: 0, MaxConcurrency: 1}
    reg.Add(w1); reg.Add(w2)
    // With MinScore 1.0, nothing qualifies
    s1 := NewScoreSchedulerWithMinScore(reg, DefaultExactMatchScorer{}, 1.0)
    if _, err := s1.PickWorker("m"); err == nil {
        t.Fatalf("expected no worker with minScore 1.0")
    }
    // With MinScore 0.0, we accept score 0 and pick least busy (w2)
    s2 := NewScoreSchedulerWithMinScore(reg, DefaultExactMatchScorer{}, 0.0)
    w, err := s2.PickWorker("m")
    if err != nil { t.Fatalf("unexpected err: %v", err) }
    if w.ID != "w2" { t.Fatalf("expected w2, got %s", w.ID) }
}

func TestAggregatedModels(t *testing.T) {
    reg := NewRegistry()
    reg.Add(&Worker{ID: "w1", Name: "Alpha", Models: map[string]bool{"llama3:8b": true, "mistral:7b": true}, MaxConcurrency: 1})
    reg.Add(&Worker{ID: "w2", Name: "Beta", Models: map[string]bool{"llama3:8b": true, "qwen2.5:14b": true}, MaxConcurrency: 1})
    list := reg.AggregatedModels()
    if len(list) != 3 { t.Fatalf("expected 3 models, got %d", len(list)) }
    found := false
    for _, m := range list {
        if m.ID == "llama3:8b" {
            found = true
            owners := strings.Join(m.Owners, ",")
            if owners != "Alpha,Beta" { t.Fatalf("owners wrong: %s", owners) }
            if m.Created <= 0 { t.Fatalf("created not set") }
        }
    }
    if !found { t.Fatalf("missing llama3:8b") }
}

func TestMetricsSnapshotBasic(t *testing.T) {
    reg := NewMetricsRegistry("v", "sha", "date", func() string { return "" })
    reg.UpsertWorker("w1", "w1", "1", "", "", 1, 0, []string{"m"})
    reg.RecordJobStart("w1")
    reg.RecordJobEnd("w1", "m", 10*time.Millisecond, 1, 2, 0, true, "")
    snap := reg.Snapshot()
    if len(snap.Workers) != 1 || snap.Server.JobsCompletedTotal != 1 {
        t.Fatalf("bad snapshot %+v", snap)
    }
}

func TestWSRegisterStoresWorkerName(t *testing.T) {
    reg := NewRegistry()
    mx := NewMetricsRegistry("test", "", "", func() string { return "" })
    srv := httptest.NewServer(WSHandler(reg, mx, "", nil))
    defer srv.Close()
    ctx := context.Background()
    wsURL := strings.Replace(srv.URL, "http", "ws", 1)
    c, _, err := websocket.Dial(ctx, wsURL, nil)
    if err != nil { t.Fatalf("dial: %v", err) }
    defer func() { _ = c.Close(websocket.StatusNormalClosure, "") }()
    rm := ctrl.RegisterMessage{Type: "register", WorkerID: "w1abcdef", WorkerName: "Alpha", Models: []string{"m"}, MaxConcurrency: 1}
    b, _ := json.Marshal(rm)
    if err := c.Write(ctx, websocket.MessageText, b); err != nil { t.Fatalf("write: %v", err) }
    for i := 0; i < 50; i++ {
        reg.mu.RLock()
        w, ok := reg.workers["w1abcdef"]
        reg.mu.RUnlock()
        if ok {
            if w.Name != "Alpha" { t.Fatalf("expected Alpha, got %s", w.Name) }
            break
        }
        time.Sleep(10 * time.Millisecond)
    }
}

func TestMetricsRegistryRace(t *testing.T) {
    reg := NewMetricsRegistry("v", "sha", "date", func() string { return "" })
    reg.UpsertWorker("w", "w", "1", "", "", 1, 0, []string{"m"})
    var wg sync.WaitGroup
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                reg.RecordHeartbeat("w")
                reg.RecordJobStart("w")
                reg.RecordJobEnd("w", "m", time.Millisecond, 0, 0, 0, true, "")
            }
        }()
    }
    wg.Wait()
}
