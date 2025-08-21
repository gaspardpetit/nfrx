package mcpcheck

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func startHTTPServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	s := server.NewMCPServer("demo-http", "1.0.0", server.WithToolCapabilities(false))
	s.AddTool(mcp.NewTool("ping"), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("pong"), nil
	})
	srv := server.NewTestStreamableHTTPServer(s)
	return srv, srv.URL + "/mcp"
}

func startSSEServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	s := server.NewMCPServer("demo-sse", "1.0.0", server.WithToolCapabilities(false))
	s.AddTool(mcp.NewTool("upper", mcp.WithString("s", mcp.Required())), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		in, _ := req.RequireString("s")
		return mcp.NewToolResultText(strings.ToUpper(in)), nil
	})
	srv := server.NewTestServer(s, server.WithStaticBasePath("/mcp"))
	return srv, srv.URL + "/mcp/sse"
}

func writeSTDIOProgram(t *testing.T, dir string) string {
	prog := `package main
import (
  "context"
  "fmt"
  "github.com/mark3labs/mcp-go/mcp"
  "github.com/mark3labs/mcp-go/server"
)
func main() {
  s := server.NewMCPServer("demo-stdio", "1.0.0", server.WithToolCapabilities(false))
  tool := mcp.NewTool("echo", mcp.WithString("msg", mcp.Required()))
  s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    msg, _ := req.RequireString("msg")
    return mcp.NewToolResultText(msg), nil
  })
  if err := server.ServeStdio(s); err != nil { fmt.Println("stdio server error:", err) }
}
`
	path := filepath.Join(dir, "server_stdio.go")
	if err := os.WriteFile(path, []byte(prog), 0o600); err != nil {
		t.Fatalf("write program: %v", err)
	}
	return path
}

func TestHTTPServerHealthy(t *testing.T) {
	srv, addr := startHTTPServer(t)
	defer srv.Close()
	checker := Configure(addr, "")
	res, err := checker.Check(context.Background())
	if err != nil || !res.Healthy || res.WorkingTransport != TransportHTTP || res.ToolsCount < 1 {
		t.Fatalf("unexpected result: %#v err=%v", res, err)
	}
}

func TestSSEServerHealthy(t *testing.T) {
	srv, addr := startSSEServer(t)
	defer srv.Close()
	checker := Configure(addr, "")
	res, err := checker.Check(context.Background())
	if err != nil || !res.Healthy || res.WorkingTransport != TransportSSE || res.ToolsCount < 1 {
		t.Fatalf("unexpected result: %#v err=%v", res, err)
	}
}

func TestSTDIODeviceHealthy(t *testing.T) {
	if raceEnabled {
		t.Skip("stdio transport races under -race")
	}
	dir := t.TempDir()
	prog := writeSTDIOProgram(t, dir)
	checker := Configure("", "go", "run", prog)
	res, err := checker.Check(context.Background())
	if err != nil || !res.Healthy || res.WorkingTransport != TransportSTDIO || res.ToolsCount < 1 {
		t.Fatalf("unexpected result: %#v err=%v", res, err)
	}
}

func TestLastGoodFallback(t *testing.T) {
	httpSrv, httpAddr := startHTTPServer(t)
	checker := Configure(httpAddr, "")
	res, err := checker.Check(context.Background())
	if err != nil || res.WorkingTransport != TransportHTTP {
		t.Fatalf("expected http success: %#v err=%v", res, err)
	}
	httpSrv.Close()

	sseSrv, sseAddr := startSSEServer(t)
	defer sseSrv.Close()
	checker.endpointURL = sseAddr
	res, err = checker.Check(context.Background())
	if err != nil || res.WorkingTransport != TransportSSE {
		t.Fatalf("expected sse fallback: %#v err=%v", res, err)
	}
}

func TestNullServerBackoff(t *testing.T) {
	checker := Configure("http://127.0.0.1:59999", "")
	_, err := checker.Check(context.Background())
	if err == nil {
		t.Fatalf("expected failure")
	}
	firstFail := checker.state.ConsecutiveFails
	_, err = checker.Check(context.Background())
	if err == nil || checker.state.ConsecutiveFails != firstFail {
		t.Fatalf("expected backoff without retry")
	}
}

func TestPartialFailure(t *testing.T) {
	srv, addr := StartPartialHTTPServer()
	defer func() { _ = srv.Close() }()
	checker := Configure(addr, "")
	res, err := checker.Check(context.Background())
	if err == nil || res.Healthy {
		t.Fatalf("expected failure")
	}
}
