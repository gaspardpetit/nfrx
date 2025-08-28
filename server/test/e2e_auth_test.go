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

    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/server/internal/adapters"
    "github.com/gaspardpetit/nfrx/server/internal/config"
    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    
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

	// bad key
	connBad, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial bad: %v", err)
	}
	regBad := ctrl.RegisterMessage{Type: "register", WorkerID: "wbad", ClientKey: "nope", Models: []string{"m"}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
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
        var v struct{ Data []struct{ ID string `json:"id"` } `json:"data"` }
        _ = json.NewDecoder(resp.Body).Decode(&v)
        _ = resp.Body.Close()
        if len(v.Data) != 0 { t.Fatalf("unexpected worker registered") }
    }

	// good key
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", ClientKey: "secret", Models: []string{"m"}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
	b, _ = json.Marshal(regMsg)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}

    // wait for registration via models API
    for i := 0; i < 50; i++ {
        r2, e2 := http.Get(srv.URL + "/api/llm/v1/models")
        if e2 == nil {
            var v struct{ Data []struct{ ID string `json:"id"` } `json:"data"` }
            _ = json.NewDecoder(r2.Body).Decode(&v)
            _ = r2.Body.Close()
            if len(v.Data) > 0 { break }
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

func TestWorkerClientKeyUnexpected(t *testing.T) {
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
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", ClientKey: "secret", Models: []string{"m"}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
	b, _ := json.Marshal(regMsg)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatalf("expected close for unexpected key")
	}
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

	connBad, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial bad: %v", err)
	}
	regBad := map[string]string{"id": "bad", "client_key": "nope"}
	b, _ := json.Marshal(regBad)
	if err := connBad.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if _, _, err := connBad.Read(ctx); err == nil {
		t.Fatalf("expected close for bad key")
	}
	_ = connBad.Close(websocket.StatusNormalClosure, "")

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	regMsg := map[string]string{"id": "good", "client_key": "secret"}
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
	conn2, _, err := websocket.Dial(ctx, wsURL2, nil)
	if err != nil {
		t.Fatalf("dial2: %v", err)
	}
	regMsg2 := map[string]string{"id": "bad", "client_key": "secret"}
	b, _ = json.Marshal(regMsg2)
	if err := conn2.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write2: %v", err)
	}
	if _, _, err := conn2.Read(ctx); err == nil {
		t.Fatalf("expected close for unexpected key (mcp)")
	}
	_ = conn2.Close(websocket.StatusNormalClosure, "")
}
