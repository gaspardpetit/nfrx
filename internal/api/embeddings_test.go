package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gaspardpetit/infero/internal/ctrl"
)

func TestEmbeddings(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	wk := &ctrl.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	reg.Add(wk)
	metricsReg := ctrl.NewMetricsRegistry("", "", "")
	h := EmbeddingsHandler(reg, sched, metricsReg, time.Second)

	go func() {
		msg := <-wk.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		ch := wk.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 200, Headers: map[string]string{"Content-Type": "application/json"}}
		ch <- ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: []byte(`{"embedding":[1]}`)}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/embeddings", strings.NewReader(`{"model":"m"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Body.String() != `{"embedding":[1]}` {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestEmbeddingsEarlyError(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	wk := &ctrl.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	reg.Add(wk)
	metricsReg := ctrl.NewMetricsRegistry("", "", "")
	h := EmbeddingsHandler(reg, sched, metricsReg, time.Second)

	go func() {
		msg := <-wk.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		ch := wk.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 502, Headers: map[string]string{"Content-Type": "application/json"}}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID, Error: &ctrl.HTTPProxyError{Code: "upstream_error", Message: "boom"}}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/embeddings", bytes.NewReader([]byte(`{"model":"m"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 502 {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Body.String() != `{"error":"upstream_error"}` {
		t.Fatalf("body %q", rec.Body.String())
	}
}
