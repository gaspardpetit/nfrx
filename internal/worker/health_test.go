package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"

	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
	"github.com/gaspardpetit/llamapool/internal/ollama"
)

// fakeHealthClient implements healthClient for unit tests.
type fakeHealthClient struct {
	models []string
	err    error
}

func (f fakeHealthClient) Health(ctx context.Context) ([]string, error) {
	return f.models, f.err
}

func TestProbeOllamaUpdatesState(t *testing.T) {
	resetState()
	cfg := config.WorkerConfig{WorkerID: "w1", WorkerName: "n", MaxConcurrency: 1}
	if err := probeOllama(context.Background(), fakeHealthClient{models: []string{"m1"}}, cfg, nil); err != nil {
		t.Fatalf("probe healthy: %v", err)
	}
	s := GetState()
	if !s.ConnectedToOllama || len(s.Models) != 1 || s.LastError != "" {
		t.Fatalf("expected healthy state, got %+v", s)
	}
	if err := probeOllama(context.Background(), fakeHealthClient{models: []string{"m1", "m2"}}, cfg, nil); err != nil {
		t.Fatalf("probe update: %v", err)
	}
	s = GetState()
	if len(s.Models) != 2 || s.Models[1] != "m2" {
		t.Fatalf("models not updated: %+v", s.Models)
	}
	if err := probeOllama(context.Background(), fakeHealthClient{err: errors.New("down")}, cfg, nil); err == nil {
		t.Fatalf("expected error")
	}
	s = GetState()
	if s.ConnectedToOllama || s.LastError == "" {
		t.Fatalf("expected failure state, got %+v", s)
	}
}

func TestProbeOllamaSendsUpdates(t *testing.T) {
	resetState()
	SetWorkerInfo("w1", "n", 1, []string{"m1"})
	SetConnectedToOllama(true)
	cfg := config.WorkerConfig{WorkerID: "w1", WorkerName: "n", MaxConcurrency: 1}

	// We now use the status update channel, and probeOllama only emits when something changes.
	ch := make(chan ctrl.StatusUpdateMessage, 1)

	// Same models: no update expected
	if err := probeOllama(context.Background(), fakeHealthClient{models: []string{"m1"}}, cfg, ch); err != nil {
		t.Fatalf("probe healthy(same models): %v", err)
	}
	select {
	case <-ch:
		t.Fatalf("unexpected update")
	default:
	}

	// Models changed: one update expected, with the new models
	if err := probeOllama(context.Background(), fakeHealthClient{models: []string{"m1", "m2"}}, cfg, ch); err != nil {
		t.Fatalf("probe healthy(update models): %v", err)
	}
	select {
	case m := <-ch:
		if len(m.Models) != 2 || m.Models[1] != "m2" {
			t.Fatalf("wrong models sent: %v", m.Models)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected update")
	}

	// Same models again: no new update expected
	if err := probeOllama(context.Background(), fakeHealthClient{models: []string{"m1", "m2"}}, cfg, ch); err != nil {
		t.Fatalf("probe healthy(no change): %v", err)
	}
	select {
	case <-ch:
		t.Fatalf("unexpected second update")
	default:
	}
}

func TestHealthProbeIntegration(t *testing.T) {
	resetState()
	var healthy atomic.Bool
	healthy.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			w.WriteHeader(404)
			return
		}
		if !healthy.Load() {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"m1"}]}`))
	}))
	defer srv.Close()
	client := ollama.New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfgFile := filepath.Join(t.TempDir(), "worker.yaml")
	cfg := config.WorkerConfig{WorkerID: "w1", WorkerName: "n", MaxConcurrency: 1}
	addr, err := StartStatusServer(ctx, "127.0.0.1:0", cfgFile, time.Second, cancel)
	if err != nil {
		t.Fatalf("start status server: %v", err)
	}
	ch := make(chan ctrl.StatusUpdateMessage, 1)
	startOllamaMonitor(ctx, cfg, client, ch, 50*time.Millisecond)

	time.Sleep(80 * time.Millisecond)
	resp, err := http.Get("http://" + addr + "/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	var st State
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()
	if !st.ConnectedToOllama {
		t.Fatalf("expected connected")
	}
	healthy.Store(false)
	time.Sleep(80 * time.Millisecond)
	resp, err = http.Get("http://" + addr + "/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()
	if st.ConnectedToOllama || st.LastError == "" {
		t.Fatalf("expected disconnected with error, got %+v", st)
	}
}
