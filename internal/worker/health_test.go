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
	probeOllama(context.Background(), fakeHealthClient{models: []string{"m1"}}, nil)
	s := GetState()
	if !s.ConnectedToOllama || len(s.Models) != 1 || s.LastError != "" {
		t.Fatalf("expected healthy state, got %+v", s)
	}
	probeOllama(context.Background(), fakeHealthClient{models: []string{"m1", "m2"}}, nil)
	s = GetState()
	if len(s.Models) != 2 || s.Models[1] != "m2" {
		t.Fatalf("models not updated: %+v", s.Models)
	}
	probeOllama(context.Background(), fakeHealthClient{err: errors.New("down")}, nil)
	s = GetState()
	if s.ConnectedToOllama || s.LastError == "" {
		t.Fatalf("expected failure state, got %+v", s)
	}
}

func TestProbeOllamaSendsUpdates(t *testing.T) {
	resetState()
	SetModels([]string{"m1"})
	ch := make(chan []string, 1)
	probeOllama(context.Background(), fakeHealthClient{models: []string{"m1"}}, ch)
	select {
	case <-ch:
		t.Fatalf("unexpected update")
	default:
	}
	probeOllama(context.Background(), fakeHealthClient{models: []string{"m1", "m2"}}, ch)
	select {
	case m := <-ch:
		if len(m) != 2 || m[1] != "m2" {
			t.Fatalf("wrong models sent: %v", m)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected update")
	}
	probeOllama(context.Background(), fakeHealthClient{models: []string{"m1", "m2"}}, ch)
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
	addr, err := StartStatusServer(ctx, "127.0.0.1:0", cfgFile, time.Second, cancel)
	if err != nil {
		t.Fatalf("start status server: %v", err)
	}
	startHealthProbe(ctx, client, 50*time.Millisecond, nil)
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
