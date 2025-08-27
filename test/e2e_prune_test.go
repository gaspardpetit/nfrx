package test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	ctrl "github.com/gaspardpetit/nfrx-sdk/contracts/control"
	"github.com/gaspardpetit/nfrx/internal/config"
	"github.com/gaspardpetit/nfrx/internal/plugin"
	"github.com/gaspardpetit/nfrx/internal/server"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
	llmext "github.com/gaspardpetit/nfrx/modules/llm/ext"
	mcpext "github.com/gaspardpetit/nfrx/modules/mcp/ext"
)

func TestHeartbeatPrune(t *testing.T) {
	cfg := config.ServerConfig{ClientKey: "secret", RequestTimeout: 5 * time.Second}
	mcp := mcpext.New(cfg, nil)
	stateReg := serverstate.NewRegistry()
	llm := llmext.New(cfg, "test", "", "", mcp.Registry(), nil)
	handler := server.New(cfg, stateReg, []plugin.Plugin{mcp, llm})
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
		if len(llm.Registry().Models()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// prune immediately
	llm.Registry().PruneExpired(0)

	if len(llm.Registry().Models()) != 0 {
		t.Fatalf("expected models pruned")
	}
}
