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

	"github.com/gaspardpetit/nfrx/internal/config"
	ctrl "github.com/gaspardpetit/nfrx/internal/ctrl"
	llmplugin "github.com/gaspardpetit/nfrx/internal/llmplugin"
	mcpbroker "github.com/gaspardpetit/nfrx/internal/mcpbroker"
	"github.com/gaspardpetit/nfrx/internal/plugin"
	"github.com/gaspardpetit/nfrx/internal/server"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

func TestWorkerModelRefresh(t *testing.T) {
	cfg := config.ServerConfig{RequestTimeout: 5 * time.Second}
	mcpReg := mcpbroker.NewRegistry(cfg.RequestTimeout)
	stateReg := serverstate.NewRegistry()
	llm := llmplugin.New(cfg, "test", "", "", mcpReg)
	handler := server.New(mcpReg, cfg, stateReg, []plugin.Plugin{llm})
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
