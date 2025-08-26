package mcpcheck

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Transport represents an MCP transport type.
type Transport string

const (
	TransportHTTP  Transport = "http"
	TransportSSE   Transport = "sse"
	TransportSTDIO Transport = "stdio"
)

// Result captures the outcome of a check.
type Result struct {
	Healthy          bool
	WorkingTransport Transport
	ToolsCount       int
	ProtocolVersion  string
	LastError        string
}

// State stores persisted information about a provider.
type State struct {
	LastOKTransport  Transport `json:"lastOKTransport"`
	ConsecutiveFails int       `json:"consecutiveFails"`
	LastError        string    `json:"lastError"`
	NextAttempt      time.Time `json:"nextAttempt"`
}

// Checker checks an MCP provider for health.
type Checker struct {
	endpointURL string
	cmd         string
	args        []string

	statePath string
	mu        sync.Mutex
	state     State
}

// Configure creates a new Checker for the given endpoint URL and/or stdio command.
func Configure(endpointURL, cmd string, args ...string) *Checker {
	key := endpointURL
	if cmd != "" {
		key = cmd + strings.Join(args, " ")
	}
	h := sha1.Sum([]byte(key))
	statePath := filepath.Join(os.TempDir(), fmt.Sprintf("mcpcheck_%x.json", h[:]))
	return &Checker{endpointURL: endpointURL, cmd: cmd, args: args, statePath: statePath}
}

// loadState loads persisted state from disk.
func (c *Checker) loadState() {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := os.ReadFile(c.statePath)
	if err == nil {
		_ = json.Unmarshal(data, &c.state)
	}
}

// saveState saves state to disk.
func (c *Checker) saveState() {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, _ := json.MarshalIndent(c.state, "", "  ")
	_ = os.WriteFile(c.statePath, data, 0o600)
}

// Check runs the health check.
func (c *Checker) Check(ctx context.Context) (Result, error) {
	c.loadState()
	if c.state.ConsecutiveFails > 0 && time.Now().Before(c.state.NextAttempt) {
		return Result{Healthy: false, LastError: c.state.LastError}, errors.New("backoff active")
	}

	var transports []Transport
	if c.state.LastOKTransport != "" {
		transports = append(transports, c.state.LastOKTransport)
	}
	for _, t := range []Transport{TransportHTTP, TransportSSE, TransportSTDIO} {
		switch t {
		case TransportHTTP, TransportSSE:
			if c.endpointURL == "" {
				continue
			}
		case TransportSTDIO:
			if c.cmd == "" {
				continue
			}
		}
		if containsTransport(transports, t) {
			continue
		}
		transports = append(transports, t)
	}

	var lastErr error
	for _, t := range transports {
		res, err := c.tryTransport(ctx, t)
		if err == nil {
			res.Healthy = true
			res.WorkingTransport = t
			c.state.LastOKTransport = t
			c.state.ConsecutiveFails = 0
			c.state.LastError = ""
			c.state.NextAttempt = time.Time{}
			c.saveState()
			return res, nil
		}
		lastErr = err
	}

	c.state.ConsecutiveFails++
	c.state.LastError = lastErr.Error()
	backoff := computeBackoff(c.state.ConsecutiveFails)
	c.state.NextAttempt = time.Now().Add(backoff)
	c.saveState()
	return Result{Healthy: false, LastError: lastErr.Error()}, lastErr
}

func containsTransport(list []Transport, t Transport) bool {
	for _, v := range list {
		if v == t {
			return true
		}
	}
	return false
}

func (c *Checker) tryTransport(ctx context.Context, t Transport) (Result, error) {
	var (
		cl  *client.Client
		err error
	)
	switch t {
	case TransportHTTP:
		cl, err = client.NewStreamableHttpClient(c.endpointURL)
	case TransportSSE:
		cl, err = client.NewSSEMCPClient(c.endpointURL)
	case TransportSTDIO:
		cl, err = client.NewStdioMCPClient(c.cmd, nil, c.args...)
	default:
		err = fmt.Errorf("unknown transport %q", t)
	}
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = cl.Close() }()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := cl.Start(ctx); err != nil {
		return Result{}, fmt.Errorf("start: %w", err)
	}
	initRes, err := cl.Initialize(ctx, mcp.InitializeRequest{})
	if err != nil {
		return Result{}, fmt.Errorf("initialize: %w", err)
	}
	tools, err := cl.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return Result{}, fmt.Errorf("tools/list: %w", err)
	}
	return Result{ToolsCount: len(tools.Tools), ProtocolVersion: initRes.ProtocolVersion}, nil
}

func computeBackoff(fails int) time.Duration {
	base := 30 * time.Second
	max := 5 * time.Minute
	d := base * time.Duration(int(math.Pow(2, float64(fails-1))))
	if d > max {
		d = max
	}
	jitter := rand.Float64()*0.4 - 0.2
	return time.Duration(float64(d) * (1 + jitter))
}

// For tests we expose a way to clear state.
func (c *Checker) ClearState() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = State{}
	_ = os.Remove(c.statePath)
}

// Internal utility for partial failure tests.
func StartPartialHTTPServer() (*http.Server, string) {
	srv := &http.Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
			ID     string `json:"id"`
		}
		_ = json.Unmarshal(body, &req)
		switch req.Method {
		case "initialize":
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"`+req.ID+`","result":{"protocolVersion":"1.0","capabilities":{},"serverInfo":{"name":"test","version":"0"}}}`)
		case "tools/list":
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"`+req.ID+`","error":{"code":-1,"message":"fail"}}`)
		default:
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"`+req.ID+`","error":{"code":-32601,"message":"unknown"}}`)
		}
	})
	srv.Handler = mux
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() { _ = srv.Serve(ln) }()
	return srv, "http://" + ln.Addr().String()
}
