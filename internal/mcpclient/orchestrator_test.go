package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
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

// fakeTransport implements transport.Interface for testing.
type fakeTransport struct {
	name     string
	calls    *[]string
	startErr error
	initErr  error
}

func (f *fakeTransport) Start(ctx context.Context) error {
	if f.calls != nil {
		*f.calls = append(*f.calls, f.name)
	}
	return f.startErr
}
func (f *fakeTransport) SendRequest(ctx context.Context, req transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	if req.Method == string(mcp.MethodInitialize) {
		if f.initErr != nil {
			return nil, f.initErr
		}
		res := mcp.InitializeResult{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION}
		b, _ := json.Marshal(res)
		return &transport.JSONRPCResponse{Result: b}, nil
	}
	return &transport.JSONRPCResponse{Result: json.RawMessage(`{}`)}, nil
}
func (f *fakeTransport) SendNotification(context.Context, mcp.JSONRPCNotification) error { return nil }
func (f *fakeTransport) SetNotificationHandler(func(mcp.JSONRPCNotification))            {}
func (f *fakeTransport) Close() error                                                    { return nil }
func (f *fakeTransport) GetSessionId() string                                            { return "" }

func TestOrchestratorFallback(t *testing.T) {
	cfg := Config{Order: []string{"stdio", "http", "oauth"}, InitTimeout: time.Millisecond}
	o := NewOrchestrator(cfg)
	var calls []string
	o.factories["stdio"] = func(Config) (*transportConnector, error) {
		return newTransportConnector(&fakeTransport{name: "stdio", calls: &calls, startErr: errors.New("boom")}, 0), nil
	}
	o.factories["http"] = func(Config) (*transportConnector, error) {
		return newTransportConnector(&fakeTransport{name: "http", calls: &calls, initErr: errors.New("init")}, 0), nil
	}
	o.factories["oauth"] = func(Config) (*transportConnector, error) {
		return newTransportConnector(&fakeTransport{name: "oauth", calls: &calls}, 0), nil
	}
	conn, err := o.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if conn == nil {
		t.Fatalf("expected connector")
	}
	want := []string{"stdio", "http", "oauth"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("order = %v want %v", calls, want)
		}
	}
}

func TestOrchestratorHTTPIntegration(t *testing.T) {
	s := server.NewMCPServer("test", "1.0", server.WithToolCapabilities(false))
	hs := server.NewStreamableHTTPServer(s)
	ts := httptest.NewServer(hs)
	defer ts.Close()

	cfg := Config{Order: []string{"http"}, InitTimeout: 5 * time.Second}
	cfg.HTTP.URL = ts.URL
	o := NewOrchestrator(cfg)
	conn, err := o.Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := conn.Close(); err != nil && !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("close: %v", err)
	}
}

func TestOrchestratorStdioIntegration(t *testing.T) {
	dir := t.TempDir()
	program := `package main
import (
"github.com/mark3labs/mcp-go/server"
)
func main(){
 s:=server.NewMCPServer("test","1.0", server.WithToolCapabilities(false))
 _=server.ServeStdio(s)
}`
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(program), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := Config{Order: []string{"stdio"}, InitTimeout: 20 * time.Second}
	cfg.Stdio.Command = "go"
	cfg.Stdio.Args = []string{"run", path}
	cfg.Stdio.AllowRelative = true
	o := NewOrchestrator(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := o.Connect(ctx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := conn.Close(); err != nil && !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("close: %v", err)
	}
}
