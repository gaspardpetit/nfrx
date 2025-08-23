package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/gaspardpetit/infx/internal/config"
	"github.com/gaspardpetit/infx/internal/ctrl"
	"github.com/gaspardpetit/infx/internal/mcp"
	"github.com/gaspardpetit/infx/internal/server"
)

func TestWorkerModelRefresh(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{RequestTimeout: 5 * time.Second}
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	handler := server.New(reg, metricsReg, sched, mcp.NewRegistry(cfg.RequestTimeout), cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/workers/connect"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	rm := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", WorkerName: "Alpha", Models: []string{"m1"}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
	b, _ := json.Marshal(rm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write register: %v", err)
	}

	waitForModels := func(n int, expect string) {
		for i := 0; i < 50; i++ {
			resp, err := http.Get(srv.URL + "/api/v1/models")
			if err == nil {
				var lr struct {
					Data []struct {
						ID string `json:"id"`
					}
				}
				if err := json.NewDecoder(resp.Body).Decode(&lr); err == nil {
					_ = resp.Body.Close()
					if len(lr.Data) == n {
						found := false
						for _, m := range lr.Data {
							if m.ID == expect {
								found = true
							}
						}
						if found {
							return
						}
					}
				} else {
					_ = resp.Body.Close()
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("models not updated")
	}

	sm := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 1, EmbeddingBatchSize: 0, Models: []string{"m1"}, Status: "idle"}
	b, _ = json.Marshal(sm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write status: %v", err)
	}

	waitForModels(1, "m1")

	sm = ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 1, EmbeddingBatchSize: 0, Models: []string{"m1", "m2"}, Status: "idle"}
	b, _ = json.Marshal(sm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write update: %v", err)
	}

	waitForModels(2, "m2")

	sm = ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 1, EmbeddingBatchSize: 0, Models: []string{"m2"}, Status: "idle"}
	b, _ = json.Marshal(sm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write update2: %v", err)
	}

	waitForModels(1, "m2")
}
