package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"sync"

	"github.com/coder/websocket"
    mcpc "github.com/gaspardpetit/nfrx/sdk/api/mcp"
    mcpcommon "github.com/gaspardpetit/nfrx/modules/mcp/common"
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
}

// NewRelayClient creates a new relay client.
func NewRelayClient(conn *websocket.Conn, providerURL, token string, timeout time.Duration) *RelayClient {
	return &RelayClient{conn: conn, providerURL: providerURL, token: token, requestTimeout: timeout, cancels: map[string]context.CancelFunc{}}
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
			if r.token != "" && f.Auth != r.token {
				_ = r.send(ctx, mcpc.Frame{T: "open.fail", SID: f.SID, Code: "MCP_UNAUTHORIZED", Msg: "unauthorized"})
				continue
			}
			_ = r.send(ctx, mcpc.Frame{T: "open.ok", SID: f.SID})
		case "rpc":
			go r.handleRPC(ctx, f)
		case "close":
			r.mu.Lock()
			if cancel, ok := r.cancels[f.SID]; ok {
				cancel()
				delete(r.cancels, f.SID)
			}
			r.mu.Unlock()
		case "ping":
			_ = r.send(ctx, mcpc.Frame{T: "pong"})
		case "pong":
			// ignore
		}
	}
}

func (r *RelayClient) handleRPC(ctx context.Context, f mcpc.Frame) {
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
	resp, err := r.callProvider(rpcCtx, f.Payload)
	if rpcCtx.Err() != nil {
		return
	}
	if err != nil {
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
	_ = r.send(ctx, mcpc.Frame{T: "close", SID: f.SID, Msg: "done"})
}

// pingLoop is now handled by agent.StartHeartbeat

func (r *RelayClient) send(ctx context.Context, f mcpc.Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return r.conn.Write(ctx, websocket.MessageText, b)
}

func (r *RelayClient) callProvider(ctx context.Context, payload []byte) ([]byte, error) {
	var env struct {
		ID any `json:"id"`
	}
	_ = json.Unmarshal(payload, &env)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.providerURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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
			"id":      env.ID,
			"error": map[string]any{
				"code":    -32000,
				"message": "Provider error",
				"data":    data,
			},
		}
		b, _ := json.Marshal(errObj)
		return b, nil
	}
	return body, nil
}
