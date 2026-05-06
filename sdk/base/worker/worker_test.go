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

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf(msg)
}

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
		"hostname":        "box1",
		"os_name":         "windows",
		"os_version":      "11",
		"backend_family":  "ollama",
		"backend_version": "0.9.6",
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
	if snap.Workers[0].HostInfo.Hostname != "box1" || snap.Workers[0].HostInfo.OSName != "windows" || snap.Workers[0].HostInfo.OSVersion != "11" || snap.Workers[0].HostInfo.WorkerVersion != "1" || snap.Workers[0].HostCPUPercent != 12.5 || snap.Workers[0].HostRAMUsedPercent != 43.75 || snap.Workers[0].InputTokensTotal != 123 || snap.Workers[0].OutputTokensTotal != 456 {
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
			"hostname":        "box1",
			"os_name":         "windows",
			"os_version":      "11",
			"backend_family":  "llama.cpp",
			"backend_version": "b4514",
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
			if w.Version == "v1.2.3" && w.HostInfo.Hostname == "box1" && w.HostInfo.BackendFamily == "llama.cpp" && w.HostInfo.BackendVersion == "b4514" && w.HostCPUPercent == 22.5 && w.HostRAMUsedPercent == 67.25 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("worker snapshot not updated: %+v", mx.Snapshot())
}

func TestStatusUpdateMergesHostInfoWithoutClearingExistingFields(t *testing.T) {
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
		AgentConfig: map[string]string{
			"hostname":   "box1",
			"os_name":    "ubuntu",
			"os_version": "24.04",
		},
	}
	b, _ := json.Marshal(rm)
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write register: %v", err)
	}
	su, _ := json.Marshal(ctrl.StatusUpdateMessage{
		Type:           "status_update",
		MaxConcurrency: 1,
		Status:         "idle",
		AgentConfig: map[string]string{
			"backend_family":  "ollama",
			"backend_version": "0.18.3",
		},
	})
	if err := c.Write(ctx, websocket.MessageText, su); err != nil {
		t.Fatalf("write status update: %v", err)
	}
	for i := 0; i < 50; i++ {
		snap := mx.Snapshot()
		if len(snap.Workers) == 1 {
			w := snap.Workers[0]
			if w.HostInfo.Hostname == "box1" && w.HostInfo.OSName == "ubuntu" && w.HostInfo.BackendFamily == "ollama" && w.HostInfo.BackendVersion == "0.18.3" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("worker snapshot not updated after status update: %+v", mx.Snapshot())
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

func TestWSHandlerDuplicateWorkerReconnectReplacesOldSession(t *testing.T) {
	reg := NewRegistry()
	mx := NewMetricsRegistry("test", "", "", func() string { return "" })
	srv := httptest.NewServer(WSHandler(reg, mx, "", nil))
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1)
	dial := func() *websocket.Conn {
		c, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return c
	}
	register := func(c *websocket.Conn, name string) {
		rm := ctrl.RegisterMessage{Type: "register", WorkerID: "wdup", WorkerName: name, Models: []string{"m"}, MaxConcurrency: 1}
		b, _ := json.Marshal(rm)
		if err := c.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatalf("write register: %v", err)
		}
	}

	conn1 := dial()
	defer func() { _ = conn1.Close(websocket.StatusNormalClosure, "") }()
	register(conn1, "first")
	waitForCondition(t, time.Second, func() bool {
		reg.mu.RLock()
		defer reg.mu.RUnlock()
		w := reg.workers["wdup"]
		return w != nil && w.Name == "first"
	}, "first worker not registered")

	conn2 := dial()
	defer func() { _ = conn2.Close(websocket.StatusNormalClosure, "") }()
	register(conn2, "second")

	waitForCondition(t, time.Second, func() bool {
		reg.mu.RLock()
		defer reg.mu.RUnlock()
		w := reg.workers["wdup"]
		return w != nil && w.Name == "second"
	}, "replacement worker not registered")

	readCtx, cancelRead := context.WithTimeout(ctx, time.Second)
	defer cancelRead()
	sawReplaced := false
	for {
		_, msg, err := conn1.Read(readCtx)
		if err != nil {
			break
		}
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg, &env); err == nil && env.Type == "session_replaced" {
			var replaced ctrl.SessionReplacedMessage
			if err := json.Unmarshal(msg, &replaced); err == nil && replaced.WorkerID == "wdup" && replaced.Reason == "duplicate_worker_id" {
				sawReplaced = true
			}
		}
	}
	if !sawReplaced {
		t.Fatalf("old worker did not receive session_replaced message")
	}

	reg.mu.RLock()
	current := reg.workers["wdup"]
	count := len(reg.workers)
	reg.mu.RUnlock()
	if current == nil || current.Name != "second" {
		t.Fatalf("unexpected current worker: %+v", current)
	}
	if count != 1 {
		t.Fatalf("expected 1 worker in registry, got %d", count)
	}

	waitForCondition(t, time.Second, func() bool {
		snap := mx.Snapshot()
		return len(snap.Workers) == 1 && snap.Workers[0].Name == "second"
	}, "replacement metrics not updated")
}

func TestWSHandlerSendsRegisterAck(t *testing.T) {
	reg := NewRegistry()
	mx := NewMetricsRegistry("test", "", "", func() string { return "" })
	srv := httptest.NewServer(WSHandler(reg, mx, "", nil))
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	rm := ctrl.RegisterMessage{Type: "register", WorkerID: "wack", WorkerName: "Ack", Models: []string{"m"}, MaxConcurrency: 2}
	b, _ := json.Marshal(rm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write register: %v", err)
	}

	readCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, msg, err := conn.Read(readCtx)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	var ack ctrl.RegisterAckMessage
	if err := json.Unmarshal(msg, &ack); err != nil {
		t.Fatalf("decode ack: %v", err)
	}
	if ack.Type != "register_ack" || ack.WorkerID != "wack" || ack.WorkerName != "Ack" || ack.MaxConcurrency != 2 {
		t.Fatalf("unexpected ack: %+v", ack)
	}
}

func TestCloseWorkerIsIdempotent(t *testing.T) {
	w := &Worker{
		ID:   "wid",
		Name: "worker",
		Send: make(chan interface{}),
		Jobs: map[string]chan interface{}{
			"job1": make(chan interface{}),
			"job2": make(chan interface{}),
		},
	}
	closeWorker(w)
	closeWorker(w)
	replaceWorker(w)
	if !w.closed {
		t.Fatalf("expected worker to remain marked closed")
	}
}
