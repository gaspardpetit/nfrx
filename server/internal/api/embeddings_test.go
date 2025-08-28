package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

    ctrl "github.com/gaspardpetit/nfrx/sdk/contracts/control"
	openai "github.com/gaspardpetit/nfrx/modules/llm/ext/openai"
	"github.com/gaspardpetit/nfrx/server/internal/adapters"
	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
)

func TestEmbeddings(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	wk := &ctrlsrv.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	reg.Add(wk)
	metricsReg := ctrlsrv.NewMetricsRegistry("", "", "")
	h := openai.EmbeddingsHandler(adapters.NewWorkerRegistry(reg), adapters.NewScheduler(sched), adapters.NewMetrics(metricsReg), time.Second, 8)

	go func() {
		msg := <-wk.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		ch := wk.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 200, Headers: map[string]string{"Content-Type": "application/json"}}
		ch <- ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: []byte(`{"embedding":[1]}`)}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/embeddings", strings.NewReader(`{"model":"m"}`))
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
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	wk := &ctrlsrv.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{})}
	reg.Add(wk)
	metricsReg := ctrlsrv.NewMetricsRegistry("", "", "")
	h := openai.EmbeddingsHandler(adapters.NewWorkerRegistry(reg), adapters.NewScheduler(sched), adapters.NewMetrics(metricsReg), time.Second, 8)

	go func() {
		msg := <-wk.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		ch := wk.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 502, Headers: map[string]string{"Content-Type": "application/json"}}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID, Error: &ctrl.HTTPProxyError{Code: "upstream_error", Message: "boom"}}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/embeddings", bytes.NewReader([]byte(`{"model":"m"}`)))
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

func TestEmbeddingsBatching(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	wk := &ctrlsrv.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{}), EmbeddingBatchSize: 1}
	reg.Add(wk)
	metricsReg := ctrlsrv.NewMetricsRegistry("", "", "")
	h := openai.EmbeddingsHandler(adapters.NewWorkerRegistry(reg), adapters.NewScheduler(sched), adapters.NewMetrics(metricsReg), time.Second, 8)

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		for i := 1; i <= 2; i++ {
			msg := <-wk.Send
			req := msg.(ctrl.HTTPProxyRequestMessage)
			var v struct {
				Input []string `json:"input"`
			}
			_ = json.Unmarshal(req.Body, &v)
			if len(v.Input) != 1 {
				errCh <- fmt.Errorf("batch size %d", len(v.Input))
				return
			}
			ch := wk.Jobs[req.RequestID]
			resp := `{"object":"list","data":[{"embedding":[` + strconv.Itoa(i) + `],"index":0}],"model":"m","usage":{"prompt_tokens":1,"total_tokens":1}}`
			ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 200, Headers: map[string]string{"Content-Type": "application/json"}}
			ch <- ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: []byte(resp)}
			ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID}
		}
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/embeddings", bytes.NewReader([]byte(`{"model":"m","input":["a","b"]}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if err := <-errCh; err != nil {
		t.Fatalf("worker: %v", err)
	}
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var resp struct {
		Data []struct {
			Embedding []int `json:"embedding"`
		} `json:"data"`
		Usage struct {
			Prompt int `json:"prompt_tokens"`
			Total  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].Embedding[0] != 1 || resp.Data[1].Embedding[0] != 2 {
		t.Fatalf("data %+v", resp.Data)
	}
	if resp.Usage.Prompt != 2 || resp.Usage.Total != 2 {
		t.Fatalf("usage %+v", resp.Usage)
	}
}

func TestEmbeddingsParallelSplit(t *testing.T) {
	reg := ctrlsrv.NewRegistry()
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	w1 := &ctrlsrv.Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{}), EmbeddingBatchSize: 10}
	w2 := &ctrlsrv.Worker{ID: "w2", Models: map[string]bool{"m": true}, MaxConcurrency: 1, Send: make(chan interface{}, 1), Jobs: make(map[string]chan interface{}), EmbeddingBatchSize: 2}
	reg.Add(w1)
	reg.Add(w2)
	metricsReg := ctrlsrv.NewMetricsRegistry("", "", "")
	h := openai.EmbeddingsHandler(adapters.NewWorkerRegistry(reg), adapters.NewScheduler(sched), adapters.NewMetrics(metricsReg), time.Second, 8)

	errCh := make(chan error, 2)
	go func() {
		msg := <-w1.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		var v struct {
			Input []string `json:"input"`
		}
		_ = json.Unmarshal(req.Body, &v)
		if len(v.Input) != 5 {
			errCh <- fmt.Errorf("w1 batch %d", len(v.Input))
			return
		}
		parts := make([]string, len(v.Input))
		for i := range v.Input {
			parts[i] = fmt.Sprintf("{\"embedding\":[%d],\"index\":%d}", i, i)
		}
		resp := fmt.Sprintf("{\"object\":\"list\",\"data\":[%s],\"model\":\"m\",\"usage\":{\"prompt_tokens\":%d,\"total_tokens\":%d}}", strings.Join(parts, ","), len(v.Input), len(v.Input))
		ch := w1.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 200, Headers: map[string]string{"Content-Type": "application/json"}}
		ch <- ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: []byte(resp)}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID}
		errCh <- nil
	}()

	go func() {
		msg := <-w2.Send
		req := msg.(ctrl.HTTPProxyRequestMessage)
		var v struct {
			Input []string `json:"input"`
		}
		_ = json.Unmarshal(req.Body, &v)
		if len(v.Input) != 1 {
			errCh <- fmt.Errorf("w2 batch %d", len(v.Input))
			return
		}
		parts := make([]string, len(v.Input))
		for i := range v.Input {
			parts[i] = fmt.Sprintf("{\"embedding\":[%d],\"index\":%d}", i, i)
		}
		resp := fmt.Sprintf("{\"object\":\"list\",\"data\":[%s],\"model\":\"m\",\"usage\":{\"prompt_tokens\":%d,\"total_tokens\":%d}}", strings.Join(parts, ","), len(v.Input), len(v.Input))
		ch := w2.Jobs[req.RequestID]
		ch <- ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: 200, Headers: map[string]string{"Content-Type": "application/json"}}
		ch <- ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: []byte(resp)}
		ch <- ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID}
		errCh <- nil
	}()

	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/embeddings", bytes.NewReader([]byte(`{"model":"m","input":["a","b","c","d","e","f"]}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if err := <-errCh; err != nil {
		t.Fatalf("worker: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("worker: %v", err)
	}
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var resp struct {
		Data []struct {
			Embedding []int `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 6 {
		t.Fatalf("want 6 got %d", len(resp.Data))
	}
}
