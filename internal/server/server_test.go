package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gaspardpetit/nfrx/internal/config"
	llmplugin "github.com/gaspardpetit/nfrx/internal/llmplugin"
	"github.com/gaspardpetit/nfrx/internal/plugin"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

func TestMetricsEndpointDefaultPort(t *testing.T) {
	cfg := config.ServerConfig{Port: 8080, MetricsAddr: ":8080", RequestTimeout: time.Second}
	mcp := mcpext.New(cfg, nil)
	stateReg := serverstate.NewRegistry()
	llm := llmplugin.New(cfg, "test", "", "", mcp.Registry(), nil)
	h := New(cfg, stateReg, []plugin.Plugin{mcp, llm})
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
	mcp := mcpext.New(cfg, nil)
	stateReg := serverstate.NewRegistry()
	llm := llmplugin.New(cfg, "test", "", "", mcp.Registry(), nil)
	h := New(cfg, stateReg, []plugin.Plugin{mcp, llm})
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
	mcp := mcpext.New(cfg, nil)
	stateReg := serverstate.NewRegistry()
	llm := llmplugin.New(cfg, "test", "", "", mcp.Registry(), nil)
	h := New(cfg, stateReg, []plugin.Plugin{mcp, llm})
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
	mcp := mcpext.New(cfg, nil)
	stateReg := serverstate.NewRegistry()
	llm := llmplugin.New(cfg, "test", "", "", mcp.Registry(), nil)
	h := New(cfg, stateReg, []plugin.Plugin{mcp, llm})
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
	mcp := mcpext.New(cfg, nil)
	stateReg := serverstate.NewRegistry()
	h := New(cfg, stateReg, []plugin.Plugin{mcp})
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/models")
	if err != nil {
		t.Fatalf("GET /api/v1/models: %v", err)
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
	llm := llmplugin.New(cfg, "test", "", "", nil, nil)
	stateReg := serverstate.NewRegistry()
	h := New(cfg, stateReg, []plugin.Plugin{llm})
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
