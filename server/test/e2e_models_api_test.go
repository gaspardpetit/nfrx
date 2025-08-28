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

    ctrl "github.com/gaspardpetit/nfrx/sdk/contracts/control"
	mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
	"github.com/gaspardpetit/nfrx/server/internal/adapters"
	"github.com/gaspardpetit/nfrx/server/internal/config"
	llm "github.com/gaspardpetit/nfrx/server/internal/llm"
	"github.com/gaspardpetit/nfrx/server/internal/plugin"
	"github.com/gaspardpetit/nfrx/server/internal/server"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

func TestModelsAPI(t *testing.T) {
	cfg := config.ServerConfig{RequestTimeout: 5 * time.Second}
	mcpPlugin := mcp.New(adapters.ServerState{}, mcp.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
	llmPlugin := llm.New(cfg, "test", "", "", mcpPlugin.Registry(), nil)
	handler := server.New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/llm/connect"

	// Worker Alpha
	connA, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial alpha: %v", err)
	}
	rmA := ctrl.RegisterMessage{Type: "register", WorkerID: "wA", WorkerName: "Alpha", Models: []string{"llama3:8b", "mistral:7b"}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
	b, _ := json.Marshal(rmA)
	if err := connA.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write alpha: %v", err)
	}

	// Worker Beta
	connB, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial beta: %v", err)
	}
	rmB := ctrl.RegisterMessage{Type: "register", WorkerID: "wB", WorkerName: "Beta", Models: []string{"llama3:8b", "qwen2.5:14b"}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
	b, _ = json.Marshal(rmB)
	if err := connB.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write beta: %v", err)
	}

	// wait for registration
	for i := 0; i < 50; i++ {
		resp, err := http.Get(srv.URL + "/api/llm/v1/models")
		if err == nil {
			var lr struct {
				Data []struct {
					ID      string `json:"id"`
					OwnedBy string `json:"owned_by"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&lr); err == nil {
				if len(lr.Data) == 3 {
					_ = resp.Body.Close()
					break
				}
			}
			_ = resp.Body.Close()
		}
		time.Sleep(20 * time.Millisecond)
	}

	resp, err := http.Get(srv.URL + "/api/llm/v1/models")
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	var list struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		_ = resp.Body.Close()
		t.Fatalf("decode list: %v", err)
	}
	_ = resp.Body.Close()
	if len(list.Data) != 3 {
		t.Fatalf("expected 3 models, got %d", len(list.Data))
	}
	for _, m := range list.Data {
		if m.ID == "llama3:8b" && m.OwnedBy != "Alpha,Beta" {
			t.Fatalf("owned_by wrong: %s", m.OwnedBy)
		}
	}

	resp, err = http.Get(srv.URL + "/api/llm/v1/models/llama3:8b")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get model: %v %d", err, resp.StatusCode)
	}
	_ = resp.Body.Close()
	resp, err = http.Get(srv.URL + "/api/llm/v1/models/doesnotexist")
	if err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing model: %v %d", err, resp.StatusCode)
	}
	_ = resp.Body.Close()

	_ = connB.Close(websocket.StatusNormalClosure, "")
	// wait for deregistration
	for i := 0; i < 50; i++ {
		resp, err := http.Get(srv.URL + "/api/llm/v1/models")
		if err == nil {
			var lr struct {
				Data []struct {
					ID      string `json:"id"`
					OwnedBy string `json:"owned_by"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&lr); err == nil {
				if len(lr.Data) == 2 {
					_ = resp.Body.Close()
					break
				}
			}
			_ = resp.Body.Close()
		}
		time.Sleep(20 * time.Millisecond)
	}

	resp, err = http.Get(srv.URL + "/api/llm/v1/models")
	if err != nil {
		t.Fatalf("list after disconnect: %v", err)
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		_ = resp.Body.Close()
		t.Fatalf("decode after disconnect: %v", err)
	}
	_ = resp.Body.Close()
	for _, m := range list.Data {
		if m.ID == "llama3:8b" && m.OwnedBy != "Alpha" {
			t.Fatalf("owned_by after disconnect: %s", m.OwnedBy)
		}
		if m.ID == "qwen2.5:14b" {
			t.Fatalf("beta model still listed")
		}
	}

	_ = connA.Close(websocket.StatusNormalClosure, "")
}
