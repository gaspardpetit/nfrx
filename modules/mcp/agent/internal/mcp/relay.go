package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sync"

	"github.com/coder/websocket"
	"github.com/gaspardpetit/nfrx/core/logx"
	mcpcommon "github.com/gaspardpetit/nfrx/modules/mcp/common"
	mcpc "github.com/gaspardpetit/nfrx/sdk/api/mcp"
	"github.com/gaspardpetit/nfrx/sdk/base/agent"
)

// RelayClient is a minimal MCP relay.
type RelayClient struct {
	conn           *websocket.Conn
	providerURL    string
	token          string
	requestTimeout time.Duration
	mu             sync.Mutex
	cancels        map[string]context.CancelFunc
	streamPref     *streamPref
}

// NewRelayClient creates a new relay client with a standalone streaming preference.
func NewRelayClient(conn *websocket.Conn, providerURL, token string, timeout time.Duration, allowStreaming bool) *RelayClient {
	return newRelayClientWithPref(conn, providerURL, token, timeout, newStreamPref(allowStreaming))
}

func newRelayClientWithPref(conn *websocket.Conn, providerURL, token string, timeout time.Duration, pref *streamPref) *RelayClient {
	if pref == nil {
		pref = newStreamPref(true)
	}
	return &RelayClient{conn: conn, providerURL: providerURL, token: token, requestTimeout: timeout, cancels: map[string]context.CancelFunc{}, streamPref: pref}
}

// Run processes frames until the context or connection ends.
func (r *RelayClient) Run(ctx context.Context) error {
	go agent.StartHeartbeat(ctx, 15*time.Second, func(c context.Context) error { return r.send(c, mcpc.Frame{T: "ping"}) })
	for {
		_, data, err := r.conn.Read(ctx)
		if err != nil {
			return err
		}
		var f mcpc.Frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		switch f.T {
		case "open":
			logx.Log.Debug().Str("sid", f.SID).Str("req_id", f.ReqID).Str("hint", f.Hint).Msg("relay received open frame")
			if r.token != "" && f.Auth != r.token {
				_ = r.send(ctx, mcpc.Frame{T: "open.fail", SID: f.SID, Code: "MCP_UNAUTHORIZED", Msg: "unauthorized"})
				logx.Log.Warn().Str("sid", f.SID).Msg("relay rejected open frame: unauthorized")
				continue
			}
			_ = r.send(ctx, mcpc.Frame{T: "open.ok", SID: f.SID})
			logx.Log.Debug().Str("sid", f.SID).Msg("relay acknowledged open frame")
		case "rpc":
			logx.Log.Debug().Str("sid", f.SID).Msg("relay received rpc frame")
			go r.handleRPC(ctx, f)
		case "close":
			logx.Log.Debug().Str("sid", f.SID).Msg("relay received close frame")
			r.mu.Lock()
			if cancel, ok := r.cancels[f.SID]; ok {
				cancel()
				delete(r.cancels, f.SID)
			}
			r.mu.Unlock()
		case "ping":
			logx.Log.Debug().Msg("relay received ping frame")
			_ = r.send(ctx, mcpc.Frame{T: "pong"})
		case "pong":
			logx.Log.Debug().Msg("relay received pong frame")
			// ignore
		}
	}
}

func (r *RelayClient) handleRPC(ctx context.Context, f mcpc.Frame) {
	logx.Log.Debug().Str("sid", f.SID).Msg("relay handling rpc frame")
	var env struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(f.Payload, &env); err != nil {
		logx.Log.Warn().Err(err).Str("sid", f.SID).Msg("relay rpc payload decode failed")
		return
	}
	rpcCtx := ctx
	var cancel context.CancelFunc
	if r.requestTimeout > 0 {
		rpcCtx, cancel = context.WithTimeout(ctx, r.requestTimeout)
	} else {
		rpcCtx, cancel = context.WithCancel(ctx)
	}
	r.mu.Lock()
	r.cancels[f.SID] = cancel
	r.mu.Unlock()
	defer func() {
		cancel()
		r.mu.Lock()
		delete(r.cancels, f.SID)
		r.mu.Unlock()
	}()
	logx.Log.Info().
		Str("sid", f.SID).
		Str("method", env.Method).
		Int("req_bytes", len(f.Payload)).
		Str("body", summarizeBody(f.Payload, 512)).
		Msg("relay forwarding rpc to provider")
	resp, err := r.callProvider(rpcCtx, f.Payload, env.ID, env.Method)
	if rpcCtx.Err() != nil {
		logx.Log.Warn().Str("sid", f.SID).Msg("relay rpc context canceled")
		return
	}
	if err != nil {
		logx.Log.Error().Err(err).Str("sid", f.SID).Msg("relay provider call failed")
		errObj := map[string]any{
			"jsonrpc": "2.0",
			"id":      nil,
			"error": map[string]any{
				"code":    -32000,
				"message": "Provider error",
				"data": map[string]any{
					"mcp": mcpcommon.ErrUpstreamError,
				},
			},
		}
		resp, _ = json.Marshal(errObj)
	}
	_ = r.send(ctx, mcpc.Frame{T: "rpc", SID: f.SID, Payload: resp})
	logx.Log.Debug().Str("sid", f.SID).Msg("relay forwarded rpc response")
	_ = r.send(ctx, mcpc.Frame{T: "close", SID: f.SID, Msg: "done"})
	logx.Log.Debug().Str("sid", f.SID).Msg("relay closed rpc session")
}

// pingLoop is now handled by agent.StartHeartbeat

func (r *RelayClient) send(ctx context.Context, f mcpc.Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return r.conn.Write(ctx, websocket.MessageText, b)
}

func (r *RelayClient) callProvider(ctx context.Context, reqPayload []byte, reqID any, method string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		allowStreaming := r.streamPref != nil && r.streamPref.Allow()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.providerURL, bytes.NewReader(reqPayload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if allowStreaming {
			req.Header.Set("Accept", "application/json, text/event-stream")
		} else {
			req.Header.Set("Accept", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		logx.Log.Debug().Str("method", method).Int("status", resp.StatusCode).Int("bytes", len(body)).Msg("provider response received")
		if resp.StatusCode == http.StatusNotAcceptable && allowStreaming && r.streamPref != nil {
			logx.Log.Warn().Str("method", method).Msg("provider rejected streaming; retrying without SSE")
			r.streamPref.Set(false)
			continue
		}
		isSSE := isSSEBody(resp.Header.Get("Content-Type"), body)
		payload := body
		if isSSE {
			if data, err := extractFirstSSEData(body); err == nil {
				payload = data
				logx.Log.Debug().Str("method", method).Int("bytes", len(payload)).Msg("parsed sse payload")
			} else {
				logx.Log.Warn().Err(err).Msg("failed parsing sse payload; forwarding raw body")
			}
		}
		if method == "tools/list" {
			logx.Log.Info().Str("method", method).Int("status", resp.StatusCode).Str("body", summarizeBody(payload, 512)).Msg("provider handshake response")
		}
		if resp.StatusCode != http.StatusOK {
			data := map[string]any{
				"mcp":    mcpcommon.ErrUpstreamError,
				"status": resp.StatusCode,
			}
			if len(body) > 0 {
				data["body"] = string(body)
			}
			errObj := map[string]any{
				"jsonrpc": "2.0",
				"id":      reqID,
				"error": map[string]any{
					"code":    -32000,
					"message": "Provider error",
					"data":    data,
				},
			}
			b, _ := json.Marshal(errObj)
			return b, nil
		}
		logProviderPayload(method, payload)
		return payload, nil
	}
	return nil, fmt.Errorf("provider rejected streaming")
}

func summarizeBody(body []byte, limit int) string {
	if limit <= 0 || len(body) <= limit {
		return string(body)
	}
	return string(body[:limit]) + "…"
}

func logProviderPayload(method string, payload []byte) {
	var env struct {
		Error *struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return
	}
	if env.Error != nil {
		logx.Log.Warn().Str("method", method).Int("code", env.Error.Code).Str("message", env.Error.Message).Str("body", summarizeBody(payload, 512)).Msg("provider returned error")
	} else {
		logx.Log.Info().Str("method", method).Str("body", summarizeBody(payload, 256)).Msg("provider returned result")
	}
}

func isSSEBody(contentType string, body []byte) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "text/event-stream") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return bytes.HasPrefix(trimmed, []byte("event:"))
}

func extractFirstSSEData(body []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)
	var data bytes.Buffer
	currentEvent := ""
	for {
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if len(line) == 0 {
			if data.Len() > 0 {
				return data.Bytes(), nil
			}
			currentEvent = ""
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(line[6:])
			if currentEvent == "close" {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(line[5:])
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(payload)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if data.Len() == 0 {
		return nil, fmt.Errorf("no sse data found")
	}
	return data.Bytes(), nil
}
