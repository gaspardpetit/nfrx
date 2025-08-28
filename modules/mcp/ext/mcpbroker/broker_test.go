package mcpbroker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
    mcpc "github.com/gaspardpetit/nfrx/sdk/api/mcp"
	mcp "github.com/gaspardpetit/nfrx/modules/mcp/agent/mcp"
	"github.com/go-chi/chi/v5"
)

type fakeState struct{}

func (fakeState) IsDraining() bool { return false }

func TestHTTPHandlerRelayOffline(t *testing.T) {
	reg := NewRegistry(time.Second, fakeState{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/id/client", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "client")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	reg.HTTPHandler()(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 got %d", rr.Code)
	}
	var resp struct {
		Error struct {
			Data struct {
				MCP string `json:"mcp"`
			}
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Data.MCP != "MCP_PROVIDER_UNAVAILABLE" {
		t.Fatalf("expected MCP_PROVIDER_UNAVAILABLE got %s", resp.Error.Data.MCP)
	}
}

func TestHTTPHandlerConcurrencyLimit(t *testing.T) {
	reg := NewRegistry(time.Second, fakeState{})
	reg.maxConc = 1
	reg.relays["client"] = &Relay{pending: map[string]chan mcpc.Frame{}, inflight: 1, methods: map[string]int{}, sessions: map[string]sessionInfo{}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/mcp/id/client", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "client")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	reg.HTTPHandler()(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 got %d", rr.Code)
	}
	var resp struct {
		Error struct {
			Data struct {
				MCP string `json:"mcp"`
			}
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Data.MCP != "MCP_LIMIT_EXCEEDED" {
		t.Fatalf("expected MCP_LIMIT_EXCEEDED got %s", resp.Error.Data.MCP)
	}
}

func TestHTTPHandlerUnauthorized(t *testing.T) {
	reg := NewRegistry(time.Second, fakeState{})
	r := chi.NewRouter()
	r.Handle("/api/mcp/connect", reg.WSHandler(""))
	r.Post("/api/mcp/id/{id}", reg.HTTPHandler())
	srv := httptest.NewServer(r)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/mcp/connect"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
	_ = conn.Write(ctx, websocket.MessageText, []byte(`{}`))
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("ack: %v", err)
	}
	var ack struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(msg, &ack)
	clientID := ack.ID

	relay := mcp.NewRelayClient(conn, "http://127.0.0.1/", "s3cr3t", time.Second)
	go func() { _ = relay.Run(ctx) }()

	req := httptest.NewRequest(http.MethodPost, "/api/mcp/id/"+clientID, bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)))
	req.Header.Set("Authorization", "Bearer wrong")
	rr := httptest.NewRecorder()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", clientID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	reg.HTTPHandler()(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", rr.Code)
	}
	var resp struct {
		Error struct {
			Data struct {
				MCP string `json:"mcp"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error.Data.MCP != "MCP_UNAUTHORIZED" {
		t.Fatalf("expected MCP_UNAUTHORIZED got %s", resp.Error.Data.MCP)
	}
}
