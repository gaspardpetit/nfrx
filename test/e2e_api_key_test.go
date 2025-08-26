package test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gaspardpetit/nfrx-sdk/config"
	"github.com/gaspardpetit/nfrx-server/internal/extension"
	llmserver "github.com/gaspardpetit/nfrx-server/internal/llmserver"
	mcpserver "github.com/gaspardpetit/nfrx-server/internal/mcpserver"
	"github.com/gaspardpetit/nfrx-server/internal/server"
	"github.com/gaspardpetit/nfrx-server/internal/serverstate"
)

func TestAPIKeyEnforcement(t *testing.T) {
	cfg := config.ServerConfig{APIKey: "test123", RequestTimeout: 5 * time.Second}
	mcp := mcpserver.New(cfg, nil)
	stateReg := serverstate.NewRegistry()
	llm := llmserver.New(cfg, "test", "", "", mcp.Registry(), nil)
	handler := server.New(cfg, stateReg, []extension.Plugin{mcp, llm})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/models")
	if err != nil {
		t.Fatalf("get without key: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test123")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get with key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
}
