package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	resp, err := rc.callProvider(context.Background(), payload, 1, "initialize")
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

func TestCallProviderParsesSSE(t *testing.T) {
	body := strings.Join([]string{
		"event: message",
		"data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}",
		"",
	}, "\n")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/json, text/event-stream" {
			t.Fatalf("expected Accept header with sse, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	rc := &RelayClient{providerURL: ts.URL, requestTimeout: time.Second, streamPref: newStreamPref(true)}
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	resp, err := rc.callProvider(context.Background(), payload, 1, "tools/list")
	if err != nil {
		t.Fatalf("callProvider: %v", err)
	}
	got := strings.TrimSpace(string(resp))
	expected := `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`
	if got != expected {
		t.Fatalf("expected %s got %s", expected, got)
	}
}
