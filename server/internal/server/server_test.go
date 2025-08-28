package server

import (
    "io"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
    "github.com/gaspardpetit/nfrx/server/internal/adapters"
    "github.com/gaspardpetit/nfrx/server/internal/config"
    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    
    "github.com/gaspardpetit/nfrx/server/internal/plugin"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

func TestMetricsEndpointDefaultPort(t *testing.T) {
	cfg := config.ServerConfig{Port: 8080, MetricsAddr: ":8080", RequestTimeout: time.Second}
	mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
    srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
    llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
	h := New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMetricsEndpointSeparatePort(t *testing.T) {
    cfg := config.ServerConfig{Port: 8080, MetricsAddr: ":9090", RequestTimeout: time.Second}
    mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
    stateReg := serverstate.NewRegistry()
    srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
    llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
	h := New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestStatePage(t *testing.T) {
    cfg := config.ServerConfig{Port: 8080, RequestTimeout: time.Second}
    mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
    stateReg := serverstate.NewRegistry()
    srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
    llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
	h := New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/state")
	if err != nil {
		t.Fatalf("GET /state: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %s", ct)
	}
}

func TestCORSAllowedOrigins(t *testing.T) {
	cfg := config.ServerConfig{Port: 8080, RequestTimeout: time.Second, AllowedOrigins: []string{"https://example.com"}}
	mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
    srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout}
    llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
	h := New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	ts := httptest.NewServer(h)
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL+"/healthz", nil)
	req.Header.Set("Origin", "https://example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if ao := resp.Header.Get("Access-Control-Allow-Origin"); ao != "https://example.com" {
		t.Fatalf("expected allowed origin header, got %q", ao)
	}

	req2, _ := http.NewRequest("GET", ts.URL+"/healthz", nil)
	req2.Header.Set("Origin", "https://evil.com")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if ao := resp2.Header.Get("Access-Control-Allow-Origin"); ao != "" {
		t.Fatalf("expected no allowed origin header, got %q", ao)
	}
}

func TestDisableLLMPlugin(t *testing.T) {
	cfg := config.ServerConfig{Port: 8080, MetricsAddr: ":8080", RequestTimeout: time.Second}
    mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
	h := New(cfg, stateReg, []plugin.Plugin{mcpPlugin})
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/llm/v1/models")
	if err != nil {
		t.Fatalf("GET /api/llm/v1/models: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	resp2, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	data, _ := io.ReadAll(resp2.Body)
	if err := resp2.Body.Close(); err != nil {
		t.Fatalf("close metrics body: %v", err)
	}
	if strings.Contains(string(data), "nfrx_server_build_info") {
		t.Fatalf("unexpected llm metrics present")
	}
}

func TestDisableMCPPlugin(t *testing.T) {
    cfg := config.ServerConfig{Port: 8080, MetricsAddr: ":8080", RequestTimeout: time.Second}
    // LLM without MCP
    srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout}
    llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
	stateReg := serverstate.NewRegistry()
	h := New(cfg, stateReg, []plugin.Plugin{llmPlugin})
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/mcp/connect")
	if err != nil {
		t.Fatalf("GET /api/mcp/connect: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}
