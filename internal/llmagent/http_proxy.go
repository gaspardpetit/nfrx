package llmagent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gaspardpetit/nfrx/internal/config"
	"github.com/gaspardpetit/nfrx/internal/ctrl"
	"github.com/gaspardpetit/nfrx/internal/logx"
	"github.com/rs/zerolog"
)

func handleHTTPProxy(ctx context.Context, cfg config.WorkerConfig, sendCh chan []byte, req ctrl.HTTPProxyRequestMessage, cancels map[string]context.CancelFunc, mu *sync.Mutex, onDone func()) {
	reqCtx, cancel := context.WithCancel(ctx)
	mu.Lock()
	cancels[req.RequestID] = cancel
	mu.Unlock()
	IncJobs()
	JobStarted()
	start := time.Now()
	success := false
	defer func() {
		cancel()
		mu.Lock()
		delete(cancels, req.RequestID)
		mu.Unlock()
		_ = DecJobs()
		onDone()
		JobCompleted(success, time.Since(start))
	}()

	logx.Log.Info().Str("request_id", req.RequestID).Msg("proxy start")
	url := cfg.CompletionBaseURL + req.Path
	if req.Stream {
		if strings.Contains(url, "?") {
			url += "&stream=true"
		} else {
			url += "?stream=true"
		}
	}
	httpReq, err := http.NewRequestWithContext(reqCtx, req.Method, url, bytes.NewReader(req.Body))
	if err != nil {
		sendProxyError(reqCtx, req.RequestID, sendCh, err)
		return
	}
	for k, v := range req.Headers {
		if strings.EqualFold(k, "Authorization") {
			continue
		}
		httpReq.Header.Set(k, v)
	}
	if cfg.CompletionAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.CompletionAPIKey)
	}
	httpReq.Header.Set("Connection", "close")

	client := &http.Client{}
	lvl := zerolog.GlobalLevel()
	if lvl <= zerolog.DebugLevel {
		logx.Log.Debug().Str("request_id", req.RequestID).Str("method", req.Method).Str("url", url).Interface("headers", httpReq.Header).Bytes("body", req.Body).Msg("http proxy request")
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		sendProxyError(reqCtx, req.RequestID, sendCh, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	hdrs := map[string]string{}
	for k, v := range resp.Header {
		hdrs[k] = strings.Join(v, ", ")
	}
	if lvl <= zerolog.DebugLevel {
		logx.Log.Debug().Str("request_id", req.RequestID).Str("url", url).Int("status", resp.StatusCode).Interface("headers", resp.Header).Msg("http proxy response")
	} else if lvl <= zerolog.InfoLevel {
		logx.Log.Info().Str("request_id", req.RequestID).Str("url", url).Int("status", resp.StatusCode).Msg("http proxy")
	}
	hmsg := ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: req.RequestID, Status: resp.StatusCode, Headers: hdrs}
	b, _ := json.Marshal(hmsg)
	sendMsg(reqCtx, sendCh, b)

	if resp.StatusCode >= http.StatusBadRequest {
		lvl := logx.Log.Warn()
		if resp.StatusCode >= http.StatusInternalServerError || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			lvl = logx.Log.Error()
		}
		lvl.Str("request_id", req.RequestID).Int("status", resp.StatusCode).Msg("proxy response")
	}

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if lvl <= zerolog.DebugLevel {
				logx.Log.Debug().Str("request_id", req.RequestID).Bytes("chunk", buf[:n]).Msg("http proxy chunk")
			}
			cmsg := ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: req.RequestID, Data: append([]byte(nil), buf[:n]...)}
			bb, _ := json.Marshal(cmsg)
			sendMsg(reqCtx, sendCh, bb)
		}
		if err != nil {
			if err == io.EOF {
				end := ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID}
				eb, _ := json.Marshal(end)
				sendMsg(reqCtx, sendCh, eb)
			} else {
				end := ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: req.RequestID, Error: &ctrl.HTTPProxyError{Code: "upstream_error", Message: err.Error()}}
				eb, _ := json.Marshal(end)
				sendMsg(reqCtx, sendCh, eb)
			}
			break
		}
	}
	success = true
	logx.Log.Info().Str("request_id", req.RequestID).Msg("proxy end")
}

func sendProxyError(ctx context.Context, id string, sendCh chan []byte, err error) {
	h := ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: id, Status: 502, Headers: map[string]string{"Content-Type": "application/json"}}
	hb, _ := json.Marshal(h)
	sendMsg(ctx, sendCh, hb)
	body := ctrl.HTTPProxyResponseChunkMessage{Type: "http_proxy_response_chunk", RequestID: id, Data: []byte(`{"error":"` + err.Error() + `"}`)}
	bb, _ := json.Marshal(body)
	sendMsg(ctx, sendCh, bb)
	end := ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: id, Error: &ctrl.HTTPProxyError{Code: "upstream_error", Message: err.Error()}}
	eb, _ := json.Marshal(end)
	sendMsg(ctx, sendCh, eb)
	logx.Log.Error().Str("request_id", id).Err(err).Msg("proxy error")
	SetLastError(err.Error())
}
