package mcp

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestStartMetricsServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, err := StartMetricsServer(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start metrics server: %v", err)
	}
	resp, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("empty response")
	}
}
