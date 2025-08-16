package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// addSSETool registers a tool that sends notifications before returning.
func addSSETool(s *server.MCPServer) {
	s.AddTool(mcp.Tool{Name: "sseTool"}, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		srv := server.ServerFromContext(ctx)
		for i := 0; i < 3; i++ {
			_ = srv.SendNotificationToClient(ctx, "test/notification", map[string]any{"i": i})
			time.Sleep(5 * time.Millisecond)
		}
		return mcp.NewToolResultText("done"), nil
	})
}

// TestCompatibility_StreamableHTTP_JSON ensures JSON responses round-trip.
func TestCompatibility_StreamableHTTP_JSON(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0", server.WithToolCapabilities(false))
	httpServer := server.NewTestStreamableHTTPServer(mcpServer)
	defer httpServer.Close()

	cfg := Config{Order: []string{"http"}, InitTimeout: 5 * time.Second}
	cfg.HTTP.URL = httpServer.URL

	conn, err := NewOrchestrator(cfg).Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	var res mcp.ListToolsResult
	if err := conn.DoRPC(context.Background(), string(mcp.MethodToolsList), mcp.ListToolsRequest{}, &res); err != nil {
		t.Fatalf("list tools: %v", err)
	}
}

// TestCompatibility_StreamableHTTP_SSE verifies notifications via SSE.
func TestCompatibility_StreamableHTTP_SSE(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0", server.WithToolCapabilities(false))
	addSSETool(mcpServer)
	httpServer := server.NewTestStreamableHTTPServer(mcpServer)
	defer httpServer.Close()

	cfg := Config{Order: []string{"http"}, InitTimeout: 5 * time.Second}
	cfg.HTTP.URL = httpServer.URL

	conn, err := NewOrchestrator(cfg).Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	tc := conn.(*transportConnector)
	var notes int
	tc.t.SetNotificationHandler(func(n mcp.JSONRPCNotification) {
		if n.Method == "test/notification" {
			notes++
		}
	})

	var res mcp.CallToolResult
	params := mcp.CallToolParams{Name: "sseTool"}
	if err := conn.DoRPC(context.Background(), string(mcp.MethodToolsCall), params, &res); err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if notes != 3 {
		t.Fatalf("got %d notifications want 3", notes)
	}
}

// TestCompatibility_ServerPush verifies push notifications over SSE GET.
func TestCompatibility_ServerPush(t *testing.T) {
	t.Skip("server push scenario not yet implemented")
}

// TestCompatibility_OAuth ensures fallback to OAuth transport.
func TestCompatibility_OAuth(t *testing.T) {
	requestCount := 0
	authHeader := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if requestCount == 0 {
			requestCount++
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if authHeader != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"protocolVersion": mcp.LATEST_PROTOCOL_VERSION}})
	}))
	defer srv.Close()

	tokenStore := transport.NewMemoryTokenStore()
	_ = tokenStore.SaveToken(&transport.Token{AccessToken: "test-token", TokenType: "Bearer"})

	cfg := Config{Order: []string{"http", "oauth"}, InitTimeout: 5 * time.Second}
	cfg.HTTP.URL = srv.URL
	cfg.OAuth.Enabled = true
	cfg.OAuth.ClientID = "client"
	cfg.OAuth.TokenURL = srv.URL // not used but required
	cfg.OAuth.Scopes = []string{"scope"}
	cfg.OAuth.TokenStore = tokenStore

	conn, err := NewOrchestrator(cfg).Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if authHeader != "Bearer test-token" {
		t.Fatalf("expected bearer token, got %q", authHeader)
	}
}

// TestCompatibility_Stdio launches a mock stdio server.
func TestCompatibility_Stdio(t *testing.T) {
	dir := t.TempDir()
	program := `package main
import (
 "os"
 "time"
 "github.com/mark3labs/mcp-go/server"
)
func main(){
 s:=server.NewMCPServer("test","1.0", server.WithToolCapabilities(false))
 go func(){time.Sleep(20*time.Millisecond); _=server.ServeStdio(s)}()
 _,_=os.Stderr.WriteString("hello stderr\n")
 select{}
}`
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(program), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := Config{Order: []string{"stdio"}, InitTimeout: 30 * time.Second}
	cfg.Stdio.Command = "go"
	cfg.Stdio.Args = []string{"run", path}
	cfg.Stdio.AllowRelative = true
	conn, err := NewOrchestrator(cfg).Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := conn.Close(); err != nil && !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("close: %v", err)
	}
}

// TestCompatibility_InvalidJSON ensures graceful failure on bad responses.
func TestCompatibility_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	cfg := Config{Order: []string{"http"}, InitTimeout: time.Second}
	cfg.HTTP.URL = srv.URL
	if _, err := NewOrchestrator(cfg).Connect(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

// TestCompatibility_TLSVerify ensures TLS verification is enforced by default.
func TestCompatibility_TLSVerify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"%s"}}`, mcp.LATEST_PROTOCOL_VERSION)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	cfg := Config{Order: []string{"http"}, InitTimeout: time.Second}
	cfg.HTTP.URL = srv.URL
	if _, err := NewOrchestrator(cfg).Connect(context.Background()); err == nil {
		t.Fatalf("expected TLS error")
	}
	cfg.HTTP.InsecureSkipVerify = true
	if _, err := NewOrchestrator(cfg).Connect(context.Background()); err != nil {
		t.Fatalf("connect with skip verify: %v", err)
	}
}
