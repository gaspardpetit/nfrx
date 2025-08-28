package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCallProviderNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte("boom"))
	}))
	defer ts.Close()
	// We don't need a real websocket connection for callProvider
	rc := &RelayClient{providerURL: ts.URL, requestTimeout: time.Second}
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	resp, err := rc.callProvider(context.Background(), payload)
	if err != nil {
		t.Fatalf("callProvider: %v", err)
	}
	var msg struct {
		Error struct {
			Data struct {
				MCP  string `json:"mcp"`
				Body string `json:"body"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Error.Data.MCP != "MCP_UPSTREAM_ERROR" {
		t.Fatalf("expected MCP_UPSTREAM_ERROR got %s", msg.Error.Data.MCP)
	}
	if msg.Error.Data.Body != "boom" {
		t.Fatalf("expected body 'boom' got %q", msg.Error.Data.Body)
	}
}
