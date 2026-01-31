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

func TestWorkerAuth(t *testing.T) {
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

	// bad key via Authorization header
	hdrBad := make(http.Header)
	hdrBad.Set("Authorization", "Bearer nope")
	connBad, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdrBad})
	if err != nil {
		t.Fatalf("dial bad: %v", err)
	}
	regBad := ctrl.RegisterMessage{Type: "register", WorkerID: "wbad", Models: []string{"m"}, MaxConcurrency: 1}
	b, _ := json.Marshal(regBad)
	if err := connBad.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	_, _, err = connBad.Read(ctx)
	if err == nil {
		t.Fatalf("expected close for bad key")
	}
	if err := connBad.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Logf("close bad: %v", err)
	}
	// ensure no models published
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/llm/v1/models", nil)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		var v struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&v)
		_ = resp.Body.Close()
		if len(v.Data) != 0 {
			t.Fatalf("unexpected worker registered")
		}
	}

	// good key via Authorization header
	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer "+cfg.ClientKey)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", Models: []string{"m"}, MaxConcurrency: 1}
	b, _ = json.Marshal(regMsg)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}

	// wait for registration via models API
	for i := 0; i < 50; i++ {
		r2, e2 := http.Get(srv.URL + "/api/llm/v1/models")
		if e2 == nil {
			var v struct {
				Data []struct {
					ID string `json:"id"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r2.Body).Decode(&v)
			_ = r2.Body.Close()
			if len(v.Data) > 0 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/api/llm/v1/models", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("models: %v %d", err, resp.StatusCode)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
}

func TestWorkerAuthNotRequiredWhenNoServerKey(t *testing.T) {
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
	// Even when a client sends an Authorization header, server without ClientKey allows connection
	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer secret")
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", Models: []string{"m"}, MaxConcurrency: 1}
	b, _ := json.Marshal(regMsg)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
	// should remain connected; close explicitly
	_ = conn.Close(websocket.StatusNormalClosure, "")
}

func TestMCPAuth(t *testing.T) {
	cfg := config.ServerConfig{ClientKey: "secret", RequestTimeout: 5 * time.Second}
	mcpPlugin := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg := serverstate.NewRegistry()
	srvOpts := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
	llmPlugin := llm.New(adapters.ServerState{}, "test", "", "", srvOpts, nil)
	handler := server.New(cfg, stateReg, []plugin.Plugin{mcpPlugin, llmPlugin})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/api/mcp/connect"

	hdrBad := make(http.Header)
	hdrBad.Set("Authorization", "Bearer nope")
	connBad, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdrBad})
	if err != nil {
		t.Fatalf("dial bad: %v", err)
	}
	regBad := map[string]string{"id": "bad"}
	b, _ := json.Marshal(regBad)
	if err := connBad.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if _, _, err := connBad.Read(ctx); err == nil {
		t.Fatalf("expected close for bad key")
	}
	_ = connBad.Close(websocket.StatusNormalClosure, "")

	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer "+cfg.ClientKey)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	regMsg := map[string]string{"id": "good"}
	b, _ = json.Marshal(regMsg)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("ack: %v", err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
	srv.Close()

	// unexpected key when server has none
	cfg = config.ServerConfig{RequestTimeout: 5 * time.Second}
	mcpReg := mcp.New(adapters.ServerState{}, nil, nil, nil, nil, nil, "test", "", "", spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}, nil)
	stateReg = serverstate.NewRegistry()
	// rebuild deps for second server
	srvOpts2 := spi.Options{RequestTimeout: cfg.RequestTimeout, ClientKey: cfg.ClientKey}
	llmPlugin = llm.New(adapters.ServerState{}, "test", "", "", srvOpts2, nil)
	handler = server.New(cfg, stateReg, []plugin.Plugin{mcpReg, llmPlugin})
	srv2 := httptest.NewServer(handler)
	defer srv2.Close()
	wsURL2 := strings.Replace(srv2.URL, "http", "ws", 1) + "/api/mcp/connect"
	// Server without ClientKey: allow connection even if client sends Authorization header
	hdr2 := make(http.Header)
	hdr2.Set("Authorization", "Bearer secret")
	conn2, _, err := websocket.Dial(ctx, wsURL2, &websocket.DialOptions{HTTPHeader: hdr2})
	if err != nil {
		t.Fatalf("dial2: %v", err)
	}
	regMsg2 := map[string]string{"id": "bad"}
	b, _ = json.Marshal(regMsg2)
	if err := conn2.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write2: %v", err)
	}
	// should remain connected; close explicitly
	_ = conn2.Close(websocket.StatusNormalClosure, "")
}
