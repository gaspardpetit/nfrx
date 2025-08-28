package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestChaosHTTP verifies the client recovers from HTTP failures without leaking resources.
func TestChaosHTTP(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1:
			// immediate 5xx
			w.WriteHeader(http.StatusInternalServerError)
		case 2:
			// stall until client timeout
			time.Sleep(150 * time.Millisecond)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"%s"}}`, mcp.LATEST_PROTOCOL_VERSION)
		}
	}))
	defer srv.Close()

	cfg := Config{Order: []string{"http"}, InitTimeout: 100 * time.Millisecond}
	cfg.HTTP.URL = srv.URL
	cfg.HTTP.Timeout = 100 * time.Millisecond

	before := runtime.NumGoroutine()

	// first attempt -> 5xx
	if _, err := NewOrchestrator(cfg).Connect(context.Background()); err == nil {
		t.Fatalf("expected error on 5xx")
	}

	// second attempt -> timeout
	if _, err := NewOrchestrator(cfg).Connect(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}

	// third attempt -> success
	cfg.InitTimeout = time.Second
	cfg.HTTP.Timeout = time.Second
	conn, err := NewOrchestrator(cfg).Connect(context.Background())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// allow goroutines to settle
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()
	if after-before > 5 {
		t.Fatalf("possible goroutine leak: before=%d after=%d", before, after)
	}
}
