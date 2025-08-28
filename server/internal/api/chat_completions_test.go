package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

    ctrl "github.com/gaspardpetit/nfrx/sdk/contracts/control"
	openai "github.com/gaspardpetit/nfrx/modules/llm/ext/openai"
	"github.com/gaspardpetit/nfrx/server/internal/adapters"
	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
)

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *flushRecorder) Flush() { f.flushed = true }

func TestChatCompletionsHeaders(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	wk := &ctrlsrv.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	reg.Add(wk)
	metricsReg := ctrlsrv.NewMetricsRegistry("", "", "")
	metricsReg.UpsertWorker("w1", "w1", "", "", "", 1, 0, []string{"m"})
	metricsReg.SetWorkerStatus("w1", ctrlsrv.StatusConnected)
	h := openai.ChatCompletionsHandler(adapters.NewWorkerRegistry(reg), adapters.NewScheduler(sched), adapters.NewMetrics(metricsReg), time.Second)

	go func() {
		msg := <-wk.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		ch := wk.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 200, Headers: map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-store"}}
		ch <- ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: []byte("data: hi\n\n")}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID}
	}()

	reqBody := `{"model":"m","stream":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %s", ct)
	}
	if !rec.flushed {
		t.Fatalf("expected flush")
	}
	if rec.Body.String() != "data: hi\n\n" {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestChatCompletionsOpaque(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	wk := &ctrlsrv.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	reg.Add(wk)
	metricsReg := ctrlsrv.NewMetricsRegistry("", "", "")
	h := openai.ChatCompletionsHandler(adapters.NewWorkerRegistry(reg), adapters.NewScheduler(sched), adapters.NewMetrics(metricsReg), time.Second)

	go func() {
		msg := <-wk.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		ch := wk.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 200, Headers: map[string]string{"Content-Type": "application/octet-stream"}}
		ch <- ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: []byte("hello ")}
		ch <- ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: []byte("world")}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/chat/completions", strings.NewReader(`{"model":"m"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Body.String() != "hello world" {
		t.Fatalf("body %q", rec.Body.String())
	}
}

func TestChatCompletionsEarlyError(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	wk := &ctrlsrv.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	reg.Add(wk)
	metricsReg := ctrlsrv.NewMetricsRegistry("", "", "")
	h := openai.ChatCompletionsHandler(adapters.NewWorkerRegistry(reg), adapters.NewScheduler(sched), adapters.NewMetrics(metricsReg), time.Second)

	go func() {
		msg := <-wk.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		ch := wk.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 502, Headers: map[string]string{"Content-Type": "application/json"}}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID, Error: &ctrl.HTTPProxyError{Code: "upstream_error", Message: "boom"}}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/chat/completions", bytes.NewReader([]byte(`{"model":"m"}`)))
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
