package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
)

func TestHandleHTTPProxyAuthAndStream(t *testing.T) {
	resetState()
	var gotAuth string
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			gotAuth = r.Header.Get("Authorization")
			if r.URL.Query().Get("stream") != "true" {
				t.Fatalf("missing stream query")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			fl := w.(http.Flusher)
			if _, err := w.Write([]byte("data: 1\n\n")); err != nil {
				t.Fatalf("write: %v", err)
			}
			fl.Flush()
			if _, err := w.Write([]byte("data: 2\n\n")); err != nil {
				t.Fatalf("write: %v", err)
			}
			fl.Flush()
			return
		}
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			if _, err := w.Write([]byte(`{"models":[{"name":"llama3"}]}`)); err != nil {
				t.Fatalf("write: %v", err)
			}
			return
		}
		w.WriteHeader(404)
	}))
	defer ollama.Close()

	cfg := config.WorkerConfig{CompletionBaseURL: ollama.URL + "/v1", CompletionAPIKey: "secret-123"}
	sendCh := make(chan []byte, 16)
	cancels := make(map[string]context.CancelFunc)
	var mu sync.Mutex
	req := ctrl.HTTPProxyRequestMessage{Type: "http_proxy_request", RequestID: "r1", Method: http.MethodPost, Path: "/chat/completions", Headers: map[string]string{"Content-Type": "application/json"}, Stream: true, Body: []byte(`{}`)}
	ctx := context.Background()
	go handleHTTPProxy(ctx, cfg, sendCh, req, cancels, &mu, func() {})

	// headers
	b := <-sendCh
	var h ctrl.HTTPProxyResponseHeadersMessage
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatalf("unmarshal headers: %v", err)
	}
	if h.Status != 200 {
		t.Fatalf("status %d", h.Status)
	}
	// collect body
	var body []byte
	for {
		b = <-sendCh
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(b, &env); err != nil {
			t.Fatalf("unmarshal env: %v", err)
		}
		if env.Type == "http_proxy_response_chunk" {
			var c ctrl.HTTPProxyResponseChunkMessage
			if err := json.Unmarshal(b, &c); err != nil {
				t.Fatalf("unmarshal chunk: %v", err)
			}
			body = append(body, c.Data...)
		} else if env.Type == "http_proxy_response_end" {
			break
		}
	}
	if string(body) != "data: 1\n\ndata: 2\n\n" {
		t.Fatalf("body %q", string(body))
	}
	if gotAuth != "Bearer secret-123" {
		t.Fatalf("auth %q", gotAuth)
	}
}
