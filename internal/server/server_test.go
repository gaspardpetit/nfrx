package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
	"github.com/gaspardpetit/llamapool/internal/mcp"
)

func TestMetricsEndpointDefaultPort(t *testing.T) {
	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{Port: 8080, RequestTimeout: time.Second}
	h := New(reg, metricsReg, sched, mcp.NewRegistry(), cfg)
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
	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{Port: 8080, MetricsPort: 9090, RequestTimeout: time.Second}
	h := New(reg, metricsReg, sched, mcp.NewRegistry(), cfg)
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
	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{Port: 8080, RequestTimeout: time.Second}
	h := New(reg, metricsReg, sched, mcp.NewRegistry(), cfg)
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
	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{Port: 8080, RequestTimeout: time.Second, AllowedOrigins: []string{"https://example.com"}}
	h := New(reg, metricsReg, sched, mcp.NewRegistry(), cfg)
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
