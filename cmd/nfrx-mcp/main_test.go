package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gaspardpetit/nfrx/internal/mcp"
)

func TestProbeProviderSetsAcceptHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json, text/event-stream" {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`))
	}))
	defer srv.Close()

	if err := mcp.ProbeProvider(context.Background(), srv.URL); err != nil {
		t.Fatalf("probeProvider returned error: %v", err)
	}
}

func TestProbeProviderReturnsBodyOnError(t *testing.T) {
	msg := "nope"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte(msg))
	}))
	defer srv.Close()
	err := mcp.ProbeProvider(context.Background(), srv.URL)
	if err == nil || !strings.Contains(err.Error(), msg) {
		t.Fatalf("expected error containing %q got %v", msg, err)
	}
}
