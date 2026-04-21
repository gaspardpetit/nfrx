package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDiscoverBackendInfoPrefersPropsBuildInfo(t *testing.T) {
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

	got := discoverBackendInfo(context.Background(), ts.URL+"/v1", "")
	if got.Family != "llama.cpp" || got.Version != "b8860-fd6ae4ca1" {
		t.Fatalf("unexpected backend info %+v", got)
	}
}

func TestDiscoverBackendInfoFallsBackToAPIVersion(t *testing.T) {
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

	got := discoverBackendInfo(context.Background(), ts.URL+"/v1", "")
	if got.Family != "ollama" || got.Version != "0.18.3" {
		t.Fatalf("unexpected backend info %+v", got)
	}
}

func TestDiscoverBackendInfoFallbackGetsFreshTimeout(t *testing.T) {
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

	got := discoverBackendInfo(context.Background(), ts.URL+"/v1", "")
	if got.Family != "ollama" || got.Version != "0.18.3" {
		t.Fatalf("unexpected backend info %+v", got)
	}
}

func TestDiscoverBackendInfoReturnsEmptyWhenUnknown(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer ts.Close()

	if got := discoverBackendInfo(context.Background(), ts.URL+"/v1", ""); got != (backendInfo{}) {
		t.Fatalf("expected empty backend info, got %+v", got)
	}
}

func TestDiscoverBackendInfoSendsBearerToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-123" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.18.3"}`))
	}))
	defer ts.Close()

	got := discoverBackendInfo(context.Background(), ts.URL, "secret-123")
	if got.Family != "ollama" || got.Version != "0.18.3" {
		t.Fatalf("unexpected backend info %+v", got)
	}
}

func TestBackendInfoFromOverrideInfersFamily(t *testing.T) {
	tests := []struct {
		name    string
		version string
		family  string
	}{
		{name: "llama.cpp", version: "llama.cpp b8860-fd6ae4ca1", family: "llama.cpp"},
		{name: "ollama", version: "ollama 0.18.3", family: "ollama"},
		{name: "unknown", version: "custom 1.0", family: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backendInfoFromOverride(tt.version)
			if got.Version != tt.version || got.Family != tt.family {
				t.Fatalf("unexpected backend info %+v", got)
			}
		})
	}
}
