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
	w1 := &Worker{ID: "w1", Labels: map[string]bool{"m": true}, InFlight: 1, MaxConcurrency: 2}
	w2 := &Worker{ID: "w2", Labels: map[string]bool{"m": true}, InFlight: 0, MaxConcurrency: 1}
	reg.Add(w1)
	reg.Add(w2)
	sched := NewScoreScheduler(reg, DefaultExactMatchScorer{})
	w, err := sched.PickWorker("m")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if w.ID != "w2" {
		t.Fatalf("expected w2, got %s", w.ID)
	}
}

func TestMinScoreThreshold(t *testing.T) {
	reg := NewRegistry()
	// Two capacity-available workers with no matching models (score 0)
	w1 := &Worker{ID: "w1", Labels: map[string]bool{}, InFlight: 1, MaxConcurrency: 2}
	w2 := &Worker{ID: "w2", Labels: map[string]bool{}, InFlight: 0, MaxConcurrency: 1}
	reg.Add(w1)
	reg.Add(w2)
	// With MinScore 1.0, nothing qualifies
	s1 := NewScoreSchedulerWithMinScore(reg, DefaultExactMatchScorer{}, 1.0)
	if _, err := s1.PickWorker("m"); err == nil {
		t.Fatalf("expected no worker with minScore 1.0")
	}
	// With MinScore 0.0, we accept score 0 and pick least busy (w2)
	s2 := NewScoreSchedulerWithMinScore(reg, DefaultExactMatchScorer{}, 0.0)
	w, err := s2.PickWorker("m")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if w.ID != "w2" {
		t.Fatalf("expected w2, got %s", w.ID)
	}
}

func TestMetricsSnapshotBasic(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date", func() string { return "" })
	reg.UpsertWorker("w1", "w1", "1", "", "", 1, 0, []string{"m"})
	reg.SetWorkerHostInfo("w1", map[string]string{
		"host_os":                  "windows",
		"host_platform":            "windows",
		"host_platform_family":     "windows",
		"host_platform_version":    "11",
		"host_kernel_version":      "10.0",
		"host_hostname":            "box1",
		"completion_agent_version": "ollama 0.9.6",
	})
	reg.RecordJobStart("w1")
	reg.RecordJobEnd("w1", "m", 10*time.Millisecond, 1, 2, 0, true, "")
	reg.RecordHeartbeat("w1", 12.5, 43.75)
	reg.AddWorkerTokens("w1", "in", 123)
	reg.AddWorkerTokens("w1", "out", 456)
	snap := reg.Snapshot()
	if len(snap.Workers) != 1 || snap.Server.JobsCompletedTotal != 1 {
		t.Fatalf("bad snapshot %+v", snap)
	}
	if snap.Workers[0].HostHostname != "box1" || snap.Workers[0].CompletionAgentVersion != "ollama 0.9.6" || snap.Workers[0].HostCPUPercent != 12.5 || snap.Workers[0].HostRAMUsedPercent != 43.75 || snap.Workers[0].InputTokensTotal != 123 || snap.Workers[0].OutputTokensTotal != 456 {
		t.Fatalf("bad host snapshot %+v", snap.Workers[0])
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
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close(websocket.StatusNormalClosure, "") }()
	rm := ctrl.RegisterMessage{Type: "register", WorkerID: "w1abcdef", WorkerName: "Alpha", Models: []string{"m"}, MaxConcurrency: 1}
	b, _ := json.Marshal(rm)
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
	for i := 0; i < 50; i++ {
		reg.mu.RLock()
		w, ok := reg.workers["w1abcdef"]
		reg.mu.RUnlock()
		if ok {
			if w.Name != "Alpha" {
				t.Fatalf("expected Alpha, got %s", w.Name)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWSRegisterAndHeartbeatStoreHostTelemetry(t *testing.T) {
	reg := NewRegistry()
	mx := NewMetricsRegistry("test", "", "", func() string { return "" })
	srv := httptest.NewServer(WSHandler(reg, mx, "", nil))
	defer srv.Close()
	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1)
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close(websocket.StatusNormalClosure, "") }()
	rm := ctrl.RegisterMessage{
		Type:           "register",
		WorkerID:       "w1abcdef",
		WorkerName:     "Alpha",
		Models:         []string{"m"},
		MaxConcurrency: 1,
		Version:        "v1.2.3",
		BuildSHA:       "abc123",
		BuildDate:      "2026-03-09",
		AgentConfig: map[string]string{
			"host_os":                  "windows",
			"host_platform":            "windows",
			"host_platform_family":     "windows",
			"host_platform_version":    "11",
			"host_kernel_version":      "10.0.26100",
			"host_hostname":            "box1",
			"completion_agent_version": "llama.cpp b4514",
		},
	}
	b, _ := json.Marshal(rm)
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
	hb, _ := json.Marshal(ctrl.HeartbeatMessage{
		Type:               "heartbeat",
		TS:                 time.Now().Unix(),
		HostCPUPercent:     22.5,
		HostRAMUsedPercent: 67.25,
	})
	if err := c.Write(ctx, websocket.MessageText, hb); err != nil {
		t.Fatalf("heartbeat write: %v", err)
	}
	for i := 0; i < 50; i++ {
		snap := mx.Snapshot()
		if len(snap.Workers) == 1 {
			w := snap.Workers[0]
			if w.Version == "v1.2.3" && w.HostHostname == "box1" && w.CompletionAgentVersion == "llama.cpp b4514" && w.HostCPUPercent == 22.5 && w.HostRAMUsedPercent == 67.25 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("worker snapshot not updated: %+v", mx.Snapshot())
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
				reg.RecordHeartbeat("w", 0, 0)
				reg.RecordJobStart("w")
				reg.RecordJobEnd("w", "m", time.Millisecond, 0, 0, 0, true, "")
			}
		}()
	}
	wg.Wait()
}
