package test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/gaspardpetit/nfrx/internal/config"
	ctrl "github.com/gaspardpetit/nfrx/internal/ctrl"
	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	mcpbroker "github.com/gaspardpetit/nfrx/internal/mcpbroker"
	"github.com/gaspardpetit/nfrx/internal/plugin"
	"github.com/gaspardpetit/nfrx/internal/server"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

func TestHeartbeatPrune(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{ClientKey: "secret", RequestTimeout: 5 * time.Second}
	metricsReg := ctrlsrv.NewMetricsRegistry("test", "", "")
	stateReg := serverstate.NewRegistry()
	handler := server.New(reg, metricsReg, sched, mcpbroker.NewRegistry(cfg.RequestTimeout), cfg, stateReg, []plugin.Plugin{})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/workers/connect"
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
