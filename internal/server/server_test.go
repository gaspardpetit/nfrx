package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
)

func TestMetricsEndpointDefaultPort(t *testing.T) {
	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{Port: 8080, RequestTimeout: time.Second, WSPath: "/api/workers/connect"}
	h := New(reg, metricsReg, sched, cfg)
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
	cfg := config.ServerConfig{Port: 8080, MetricsPort: 9090, RequestTimeout: time.Second, WSPath: "/api/workers/connect"}
	h := New(reg, metricsReg, sched, cfg)
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

func TestStatusPage(t *testing.T) {
	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{Port: 8080, RequestTimeout: time.Second, WSPath: "/api/workers/connect"}
	h := New(reg, metricsReg, sched, cfg)
	ts := httptest.NewServer(h)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %s", ct)
	}
}
