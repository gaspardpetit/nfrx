package test

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/server/internal/adapters"
    "github.com/gaspardpetit/nfrx/server/internal/api"
    "github.com/gaspardpetit/nfrx/server/internal/config"
    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
    "github.com/gaspardpetit/nfrx/server/internal/plugin"
    "github.com/gaspardpetit/nfrx/server/internal/server"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
    "github.com/gaspardpetit/nfrx/sdk/spi"
)

func TestAPIKeyEnforcement(t *testing.T) {
	cfg := config.ServerConfig{APIKey: "test123", RequestTimeout: 5 * time.Second}
	mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
    reg := ctrlsrv.NewRegistry()
    metricsReg := ctrlsrv.NewMetricsRegistry("test", "", "")
    sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
    connect := ctrlsrv.WSHandler(reg, metricsReg, cfg.ClientKey)
    wr := adapters.NewWorkerRegistry(reg)
    sc := adapters.NewScheduler(sched)
    mx := adapters.NewMetrics(metricsReg)
    stateProvider := func() any { return metricsReg.Snapshot() }
    authMW := api.APIKeyMiddleware(cfg.APIKey)
    srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey, MaxParallelEmbeddings: cfg.MaxParallelEmbeddings}
    llmPlugin := llm.New(adapters.ServerState{}, connect, wr, sc, mx, stateProvider, "test", "", "", srvOpts, authMW)
	handler := server.New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/llm/v1/models")
	if err != nil {
		t.Fatalf("get without key: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/llm/v1/models", nil)
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
