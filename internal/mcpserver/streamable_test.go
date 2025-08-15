package mcpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInitialize(t *testing.T) {
	handler := NewHandler()
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	reqBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1.0"}}`)
	resp, err := http.Post(srv.URL+"/mcp", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if sid := resp.Header.Get("Mcp-Session-Id"); sid == "" {
		t.Fatalf("missing session id")
	}
	var js map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&js); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if js["result"] == nil {
		t.Fatalf("missing result")
	}
}

func TestListenSSE(t *testing.T) {
	handler := NewHandler()
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	reqBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1.0"}}`)
	resp, err := http.Post(srv.URL+"/mcp", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	_ = resp.Body.Close()
	sid := resp.Header.Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatalf("missing session id")
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/mcp", nil)
	req.Header.Set("Mcp-Session-Id", sid)
	client := &http.Client{}
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ct := resp2.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected sse, got %s", ct)
	}
	_ = resp2.Body.Close()
}
