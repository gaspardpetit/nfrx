package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHTTPHandlerRelayOffline(t *testing.T) {
	reg := NewRegistry()
	reg.allowed = map[string]bool{"client": true}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp/client", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("client_id", "client")
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
	reg := NewRegistry()
	reg.allowed = map[string]bool{"client": true}
	reg.maxConc = 1
	reg.relays["client"] = &Relay{pending: map[string]chan Frame{}, inflight: 1}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp/client", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("client_id", "client")
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
