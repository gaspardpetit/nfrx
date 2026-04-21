package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDiscoverCompletionAgentVersionPrefersPropsBuildInfo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/props":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"build_info":"b8860-fd6ae4ca1"}`))
		case "/api/version":
			t.Fatalf("unexpected fallback request to %s", r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	got := discoverCompletionAgentVersion(context.Background(), ts.URL+"/v1", "")
	if got != "llama.cpp b8860-fd6ae4ca1" {
		t.Fatalf("unexpected version %q", got)
	}
}

func TestDiscoverCompletionAgentVersionFallsBackToAPIVersion(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/props":
			http.NotFound(w, r)
		case "/api/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"0.18.3"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	got := discoverCompletionAgentVersion(context.Background(), ts.URL+"/v1", "")
	if got != "ollama 0.18.3" {
		t.Fatalf("unexpected version %q", got)
	}
}

func TestDiscoverCompletionAgentVersionFallbackGetsFreshTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/props":
			time.Sleep(backendVersionProbeTimeout + 200*time.Millisecond)
			http.NotFound(w, r)
		case "/api/version":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"0.18.3"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	got := discoverCompletionAgentVersion(context.Background(), ts.URL+"/v1", "")
	if got != "ollama 0.18.3" {
		t.Fatalf("unexpected version %q", got)
	}
}

func TestDiscoverCompletionAgentVersionReturnsEmptyWhenUnknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	if got := discoverCompletionAgentVersion(context.Background(), ts.URL+"/v1", ""); got != "" {
		t.Fatalf("expected empty version, got %q", got)
	}
}

func TestDiscoverCompletionAgentVersionSendsBearerToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-123" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.18.3"}`))
	}))
	defer ts.Close()

	got := discoverCompletionAgentVersion(context.Background(), ts.URL, "secret-123")
	if got != "ollama 0.18.3" {
		t.Fatalf("unexpected version %q", got)
	}
}
