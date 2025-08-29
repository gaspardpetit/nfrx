package test

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
    wp "github.com/gaspardpetit/nfrx/sdk/base/agent/workerproxy"
    "github.com/gaspardpetit/nfrx/server/internal/adapters"
    "github.com/gaspardpetit/nfrx/server/internal/config"
    "github.com/gaspardpetit/nfrx/server/internal/plugin"
    "github.com/gaspardpetit/nfrx/server/internal/server"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

func TestE2EChatCompletionsProxy_NonStreaming(t *testing.T) {
    cfg := config.ServerConfig{ClientKey: "secret", APIKey: "apikey", RequestTimeout: 5 * time.Second}
    mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
    stateReg := serverstate.NewRegistry()
    srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
    llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
    handler := server.New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
    srv := httptest.NewServer(handler)
    defer srv.Close()

    // Upstream stub: reports a single model and returns JSON for non-streaming completions
    ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/api/tags":
            w.Header().Set("Content-Type", "application/json")
            _, _ = w.Write([]byte(`{"models":[{"name":"llama3"}]}`))
        case "/v1/chat/completions":
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(200)
            _, _ = w.Write([]byte(`{"id":"cmpl","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`))
        default:
            w.WriteHeader(404)
        }
    }))
    defer ollama.Close()

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/llm/connect"
    go func() {
        probe := func(pctx context.Context) (wp.ProbeResult, error) { return wp.ProbeResult{Ready: true, Models: []string{"llama3"}, MaxConcurrency: 2}, nil }
        _ = wp.Run(ctx, wp.Config{ServerURL: wsURL, ClientKey: "secret", BaseURL: ollama.URL + "/v1", ProbeFunc: probe, ProbeInterval: 50 * time.Millisecond, ClientID: "w1", ClientName: "w1", MaxConcurrency: 2})
    }()

    // wait for worker registration
    for i := 0; i < 20; i++ {
        if resp, err := http.Get(srv.URL + "/api/llm/v1/models"); err == nil {
            var v struct{ Data []struct{ ID string `json:"id"` } `json:"data"` }
            if json.NewDecoder(resp.Body).Decode(&v) == nil {
                _ = resp.Body.Close()
                if len(v.Data) > 0 { break }
            } else { _ = resp.Body.Close() }
        }
        time.Sleep(100 * time.Millisecond)
    }

    // Non-streaming request
    reqBody := []byte(`{"model":"llama3","stream":false}`)
    req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/llm/v1/chat/completions", bytes.NewReader(reqBody))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer apikey")
    resp, err := http.DefaultClient.Do(req)
    if err != nil { t.Fatalf("request: %v", err) }
    defer func() { _ = resp.Body.Close() }()
    if resp.StatusCode != 200 { t.Fatalf("status %d", resp.StatusCode) }
    if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
        t.Fatalf("content-type %s", ct)
    }
    b, _ := io.ReadAll(resp.Body)
    expected := `{"id":"cmpl","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`
    if string(b) != expected { t.Fatalf("body %q", string(b)) }
}

