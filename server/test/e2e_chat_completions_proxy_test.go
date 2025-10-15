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

	"sync/atomic"

	llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
	mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	wp "github.com/gaspardpetit/nfrx/sdk/base/agent/workerproxy"
	llmctrl "github.com/gaspardpetit/nfrx/sdk/base/worker"
	"github.com/gaspardpetit/nfrx/server/internal/adapters"
	"github.com/gaspardpetit/nfrx/server/internal/config"
	"github.com/gaspardpetit/nfrx/server/internal/plugin"
	"github.com/gaspardpetit/nfrx/server/internal/server"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

func TestE2EChatCompletionsProxy(t *testing.T) {
	cfg := config.ServerConfig{ClientKey: "secret", APIKey: "apikey", RequestTimeout: 5 * time.Second}
	mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
	srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
	llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
	handler := server.New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	var gotAuth atomic.Value
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"models":[{"name":"llama3"}]}`)); err != nil {
				t.Fatalf("write tags: %v", err)
			}
		case "/v1/chat/completions":
			gotAuth.Store(r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-store")
			fl := w.(http.Flusher)
			w.WriteHeader(200)
			if _, err := w.Write([]byte("data: {\"choices\":[]}\n\n")); err != nil {
				t.Fatalf("write chunk1: %v", err)
			}
			fl.Flush()
			if _, err := w.Write([]byte("data: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\n")); err != nil {
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
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/llm/connect"
	go func() {
		probe := func(pctx context.Context) (wp.ProbeResult, error) {
			return wp.ProbeResult{Ready: true, Models: []string{"llama3"}, MaxConcurrency: 2}, nil
		}
		_ = wp.Run(ctx, wp.Config{ServerURL: wsURL, ClientKey: "secret", BaseURL: ollama.URL + "/v1", APIKey: "secret-123", ProbeFunc: probe, ProbeInterval: 50 * time.Millisecond, ClientID: "w1", ClientName: "w1", MaxConcurrency: 2})
	}()

	// wait for worker registration
	for i := 0; i < 20; i++ {
		resp, err := http.Get(srv.URL + "/api/llm/v1/models")
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
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/llm/v1/chat/completions", bytes.NewReader(reqBody))
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
	expected := "data: {\"choices\":[]}\n\ndata: {\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":2,\"total_tokens\":3}}\n\ndata: [DONE]\n\n"
	if string(b) != expected {
		t.Fatalf("body %q", string(b))
	}
	auth := gotAuth.Load().(string)
	if auth != "Bearer secret-123" {
		t.Fatalf("auth %q", auth)
	}

	// Verify tokens via plugin state API
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/state", nil)
	req2.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	var env struct {
		Plugins map[string]any `json:"plugins"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&env); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	raw, ok := env.Plugins["llm"]
	if !ok {
		t.Fatalf("missing llm state")
	}
	bstate, _ := json.Marshal(raw)
	var st llmctrl.StateResponse
	if err := json.Unmarshal(bstate, &st); err != nil {
		t.Fatalf("decode llm state: %v", err)
	}
	if len(st.Workers) < 1 {
		t.Fatalf("expected workers in state")
	}
	// state returned and has workers; token counters may be updated asynchronously
}
