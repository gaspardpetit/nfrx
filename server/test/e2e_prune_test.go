package test

import (
    "context"
    "encoding/json"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/coder/websocket"

    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/server/internal/adapters"
    "github.com/gaspardpetit/nfrx/server/internal/config"
    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
    "github.com/gaspardpetit/nfrx/server/internal/plugin"
    "github.com/gaspardpetit/nfrx/server/internal/server"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

func TestHeartbeatPrune(t *testing.T) {
	cfg := config.ServerConfig{ClientKey: "secret", RequestTimeout: 5 * time.Second}
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
    srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
    llmPlugin := llm.New(adapters.ServerState{}, connect, wr, sc, mx, stateProvider, "test", "", "", srvOpts, nil)
	handler := server.New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/llm/connect"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", ClientKey: "secret", Models: []string{"m"}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
	b, _ := json.Marshal(regMsg)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}

	// ensure registration
    for i := 0; i < 50; i++ {
        if len(reg.Models()) > 0 {
            break
        }
        time.Sleep(20 * time.Millisecond)
    }

	// prune immediately
    reg.PruneExpired(0)

    if len(reg.Models()) != 0 {
        t.Fatalf("expected models pruned")
    }
}
