package test

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "sync/atomic"
    "testing"
    "time"

    "github.com/gaspardpetit/nfrx/modules/llm/agent/worker"
    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/server/internal/adapters"
    "github.com/gaspardpetit/nfrx/server/internal/config"
    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    
    "github.com/gaspardpetit/nfrx/server/internal/plugin"
    "github.com/gaspardpetit/nfrx/server/internal/server"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

func TestE2EEmbeddingsProxy(t *testing.T) {
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
		case "/v1/embeddings":
			gotAuth.Store(r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"data":[{"embedding":[1,2,3],"index":0}],"model":"llama3","usage":{"prompt_tokens":1,"total_tokens":1}}`)); err != nil {
				t.Fatalf("write embeddings: %v", err)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer ollama.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/llm/connect"
	go func() {
		_ = worker.Run(ctx, worker.Config{ServerURL: wsURL, ClientKey: "secret", CompletionBaseURL: ollama.URL + "/v1", CompletionAPIKey: "secret-123", ClientID: "w1", ClientName: "w1", MaxConcurrency: 2, EmbeddingBatchSize: 0})
	}()

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

	reqBody := []byte(`{"model":"llama3","input":"hi"}`)
    req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/llm/v1/embeddings", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer apikey")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type %s", ct)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(b), "embedding") {
		t.Fatalf("body %q", string(b))
	}
	auth := gotAuth.Load().(string)
	if auth != "Bearer secret-123" {
		t.Fatalf("auth %q", auth)
	}
}
