package workerproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
)

func TestHandleHTTPProxy_AuthAndStreaming(t *testing.T) {
	// Upstream that checks Authorization and streams SSE-like chunks
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stream" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		fl, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: 1\n\n"))
		if fl != nil {
			fl.Flush()
		}
		_, _ = w.Write([]byte("data: 2\n\n"))
		if fl != nil {
			fl.Flush()
		}
	}))
	defer upstream.Close()

	cfg := Config{BaseURL: upstream.URL, APIKey: "secret-123"}
	sendCh := make(chan []byte, 16)
	cancels := make(map[string]context.CancelFunc)
	var mu sync.Mutex
	req := ctrl.HTTPProxyRequestMessage{Type: "http_proxy_request", RequestID: "r1", Method: http.MethodGet, Path: "/stream", Headers: map[string]string{"Accept": "text/event-stream"}, Stream: true}
	ctx := context.Background()
	go handleHTTPProxy(ctx, cfg, sendCh, req, cancels, &mu, func() {})

	// Read headers
	b := <-sendCh
	var h ctrl.HTTPProxyResponseHeadersMessage
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatalf("unmarshal headers: %v", err)
	}
	if h.Status != 200 {
		t.Fatalf("status %d", h.Status)
	}
	// Collect body
	var body []byte
	for {
		b = <-sendCh
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(b, &env); err != nil {
			t.Fatalf("unmarshal env: %v", err)
		}
		switch env.Type {
		case "http_proxy_response_chunk":
			var c ctrl.HTTPProxyResponseChunkMessage
			if err := json.Unmarshal(b, &c); err != nil {
				t.Fatalf("unmarshal chunk: %v", err)
			}
			body = append(body, c.Data...)
		case "http_proxy_response_end":
			goto done
		}
	}
done:
	if string(body) != "data: 1\n\ndata: 2\n\n" {
		t.Fatalf("body %q", string(body))
	}
	if gotAuth != "Bearer secret-123" {
		t.Fatalf("auth %q", gotAuth)
	}
}
