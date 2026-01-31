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

	llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
	mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
	"github.com/gaspardpetit/nfrx/server/internal/adapters"
	"github.com/gaspardpetit/nfrx/server/internal/config"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	"github.com/gaspardpetit/nfrx/server/internal/plugin"
	"github.com/gaspardpetit/nfrx/server/internal/server"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

func TestWorkerModelRefresh(t *testing.T) {
	cfg := config.ServerConfig{RequestTimeout: 5 * time.Second}
	mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
	srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
	llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
	handler := server.New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/llm/connect"
	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer "+cfg.ClientKey)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	rm := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", WorkerName: "Alpha", Models: []string{"m1"}, MaxConcurrency: 1}
	b, _ := json.Marshal(rm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write register: %v", err)
	}

	waitForModels := func(n int, expect string) {
		for i := 0; i < 50; i++ {
			resp, err := http.Get(srv.URL + "/api/llm/v1/models")
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

	sm := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 1, Models: []string{"m1"}, Status: "idle"}
	b, _ = json.Marshal(sm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write status: %v", err)
	}

	waitForModels(1, "m1")

	sm = ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 1, Models: []string{"m1", "m2"}, Status: "idle"}
	b, _ = json.Marshal(sm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write update: %v", err)
	}

	waitForModels(2, "m2")

	sm = ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 1, Models: []string{"m2"}, Status: "idle"}
	b, _ = json.Marshal(sm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write update2: %v", err)
	}

	waitForModels(1, "m2")
}
