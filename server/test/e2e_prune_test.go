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
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	"github.com/gaspardpetit/nfrx/server/internal/adapters"
	"github.com/gaspardpetit/nfrx/server/internal/config"
	"github.com/gaspardpetit/nfrx/server/internal/plugin"
	"github.com/gaspardpetit/nfrx/server/internal/server"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

func TestDisconnectRemovesModels(t *testing.T) {
	cfg := config.ServerConfig{ClientKey: "secret", RequestTimeout: 5 * time.Second}
	mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
	srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
	llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
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

	// ensure registration visible via public API
	for i := 0; i < 100; i++ {
		resp, err := http.Get(srv.URL + "/api/llm/v1/models")
		if err == nil {
			var v struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&v)
			_ = resp.Body.Close()
			if len(v.Data) > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	// disconnect
	_ = conn.Close(websocket.StatusNormalClosure, "")
	// wait until models list is empty
	for i := 0; i < 100; i++ {
		resp, err := http.Get(srv.URL + "/api/llm/v1/models")
		if err == nil {
			var v struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&v)
			_ = resp.Body.Close()
			if len(v.Data) == 0 {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected models to be removed after disconnect")
}

func TestPruneLastWorkerTogglesNotReady(t *testing.T) {
	cfg := config.ServerConfig{ClientKey: "secret", RequestTimeout: 5 * time.Second}
	// Faster heartbeat/prune for the test
	fast := spi.Options{
		RequestTimeout:         cfg.RequestTimeout,
		ClientKey:              cfg.ClientKey,
		AgentHeartbeatInterval: 20 * time.Millisecond,
		AgentHeartbeatExpiry:   50 * time.Millisecond,
	}
	mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", fast, nil)
	stateReg := serverstate.NewRegistry()
	llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", fast, nil)
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
	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w2", ClientKey: "secret", Models: []string{"m"}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
	b, _ := json.Marshal(regMsg)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait for server to flip to ready after first worker registers
	for i := 0; i < 100; i++ {
		if serverstate.GetState() == "ready" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Do not send heartbeats; allow prune to evict the lone worker, then assert not_ready
	for i := 0; i < 200; i++ {
		if serverstate.GetState() == "not_ready" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected server state to become not_ready after prune; got %q", serverstate.GetState())
}
