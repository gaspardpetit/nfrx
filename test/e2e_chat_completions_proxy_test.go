package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
	"github.com/gaspardpetit/llamapool/internal/mcp"
	"github.com/gaspardpetit/llamapool/internal/server"
	"github.com/gaspardpetit/llamapool/internal/worker"
	"sync/atomic"
)

func TestE2EChatCompletionsProxy(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{WorkerKey: "secret", APIKey: "apikey", RequestTimeout: 5 * time.Second}
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	handler := server.New(reg, metricsReg, sched, mcp.NewRegistry(), cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	var gotAuth atomic.Value
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"models":[{"name":"llama3"}]}`)); err != nil {
				t.Fatalf("write tags: %v", err)
			}
		case r.URL.Path == "/v1/chat/completions" && r.URL.Query().Get("stream") == "true":
			gotAuth.Store(r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-store")
			fl := w.(http.Flusher)
			w.WriteHeader(200)
			if _, err := w.Write([]byte("data: 1\n\n")); err != nil {
				t.Fatalf("write chunk1: %v", err)
			}
			fl.Flush()
			if _, err := w.Write([]byte("data: 2\n\n")); err != nil {
				t.Fatalf("write chunk2: %v", err)
			}
			fl.Flush()
			if _, err := w.Write([]byte("data: [DONE]\n\n")); err != nil {
				t.Fatalf("write done: %v", err)
			}
			fl.Flush()
		default:
			w.WriteHeader(404)
		}
	}))
	defer ollama.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/workers/connect"
	go func() {
		_ = worker.Run(ctx, config.WorkerConfig{ServerURL: wsURL, WorkerKey: "secret", OllamaBaseURL: ollama.URL, OllamaAPIKey: "secret-123", WorkerID: "w1", WorkerName: "w1", MaxConcurrency: 2})
	}()

	// wait for worker registration
	for i := 0; i < 20; i++ {
		resp, err := http.Get(srv.URL + "/api/v1/models")
		if err == nil {
			var v struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&v); err == nil {
				if len(v.Data) > 0 {
					_ = resp.Body.Close()
					break
				}
			}
			_ = resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	reqBody := []byte(`{"model":"llama3","stream":true}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer apikey")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %s", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control %s", cc)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read body: %v", err)
	}
	expected := "data: 1\n\ndata: 2\n\ndata: [DONE]\n\n"
	if string(b) != expected {
		t.Fatalf("body %q", string(b))
	}
	auth := gotAuth.Load().(string)
	if auth != "Bearer secret-123" {
		t.Fatalf("auth %q", auth)
	}
}
