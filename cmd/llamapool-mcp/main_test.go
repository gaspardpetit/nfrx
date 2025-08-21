package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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

	if err := probeProvider(context.Background(), srv.URL); err != nil {
		t.Fatalf("probeProvider returned error: %v", err)
	}
}
