package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// RelayClient is a minimal MCP relay.
type RelayClient struct {
	conn           *websocket.Conn
	providerURL    string
	token          string
	requestTimeout time.Duration
}

// NewRelayClient creates a new relay client.
func NewRelayClient(conn *websocket.Conn, providerURL, token string, timeout time.Duration) *RelayClient {
	return &RelayClient{conn: conn, providerURL: providerURL, token: token, requestTimeout: timeout}
}

// Run processes frames until the context or connection ends.
func (r *RelayClient) Run(ctx context.Context) error {
	go r.pingLoop(ctx)
	for {
		_, data, err := r.conn.Read(ctx)
		if err != nil {
			return err
		}
		var f Frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		switch f.T {
		case "open":
			if r.token != "" && f.Auth != r.token {
				_ = r.send(ctx, Frame{T: "open.fail", SID: f.SID, Code: "MCP_UNAUTHORIZED", Msg: "unauthorized"})
				continue
			}
			_ = r.send(ctx, Frame{T: "open.ok", SID: f.SID})
		case "rpc":
			go r.handleRPC(ctx, f)
		case "ping":
			_ = r.send(ctx, Frame{T: "pong"})
		case "pong":
			// ignore
		}
	}
}

func (r *RelayClient) handleRPC(ctx context.Context, f Frame) {
	rpcCtx := ctx
	var cancel context.CancelFunc
	if r.requestTimeout > 0 {
		rpcCtx, cancel = context.WithTimeout(ctx, r.requestTimeout)
	}
	resp, err := r.callProvider(rpcCtx, f.Payload)
	if cancel != nil {
		cancel()
	}
	if err != nil {
		errObj := map[string]any{
			"jsonrpc": "2.0",
			"id":      nil,
			"error": map[string]any{
				"code":    -32000,
				"message": "Provider error",
				"data": map[string]any{
					"mcp": "MCP_UPSTREAM_ERROR",
				},
			},
		}
		resp, _ = json.Marshal(errObj)
	}
	_ = r.send(ctx, Frame{T: "rpc", SID: f.SID, Payload: resp})
	_ = r.send(ctx, Frame{T: "close", SID: f.SID, Msg: "done"})
}

func (r *RelayClient) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = r.send(ctx, Frame{T: "ping"})
		case <-ctx.Done():
			return
		}
	}
}

func (r *RelayClient) send(ctx context.Context, f Frame) error {
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
		errObj := map[string]any{
			"jsonrpc": "2.0",
			"id":      env.ID,
			"error": map[string]any{
				"code":    -32000,
				"message": "Provider error",
				"data": map[string]any{
					"mcp":    "MCP_UPSTREAM_ERROR",
					"status": resp.StatusCode,
				},
			},
		}
		b, _ := json.Marshal(errObj)
		return b, nil
	}
	return body, nil
}
