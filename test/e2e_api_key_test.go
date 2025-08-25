package test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gaspardpetit/nfrx/internal/config"
	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	mcpbroker "github.com/gaspardpetit/nfrx/internal/mcpbroker"
	"github.com/gaspardpetit/nfrx/internal/plugin"
	"github.com/gaspardpetit/nfrx/internal/server"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

func TestAPIKeyEnforcement(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{APIKey: "test123", RequestTimeout: 5 * time.Second}
	metricsReg := ctrlsrv.NewMetricsRegistry("test", "", "")
	stateReg := serverstate.NewRegistry()
	handler := server.New(reg, metricsReg, sched, mcpbroker.NewRegistry(cfg.RequestTimeout), cfg, stateReg, []plugin.Plugin{})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/models")
	if err != nil {
		t.Fatalf("get without key: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test123")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get with key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
}
