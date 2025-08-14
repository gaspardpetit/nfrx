package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
)

func TestMetricsEndpointDefaultPort(t *testing.T) {
	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{Port: 8080, WSPath: "/workers", RequestTimeout: time.Second}
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
	cfg := config.ServerConfig{Port: 8080, MetricsPort: 9090, WSPath: "/workers", RequestTimeout: time.Second}
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
