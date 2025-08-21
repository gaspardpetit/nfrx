package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// RelayClient is a minimal MCP relay.
type RelayClient struct {
	conn        *websocket.Conn
	providerURL string
	token       string
	writeMu     sync.Mutex
	sessMu      sync.Mutex
	sessions    map[string]context.CancelFunc
}

// NewRelayClient creates a new relay client.
func NewRelayClient(conn *websocket.Conn, providerURL, token string) *RelayClient {
	return &RelayClient{conn: conn, providerURL: providerURL, token: token, sessions: map[string]context.CancelFunc{}}
}

// Run processes frames until the context or connection ends.
func (r *RelayClient) Run(ctx context.Context) error {
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
			sessionCtx, cancel := context.WithCancel(ctx)
			r.sessMu.Lock()
			r.sessions[f.SID] = cancel
			r.sessMu.Unlock()
			go func(sid string, payload []byte) {
				defer func() {
					r.sessMu.Lock()
					delete(r.sessions, sid)
					r.sessMu.Unlock()
				}()
				resp, err := r.callProvider(sessionCtx, payload)
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
				_ = r.send(ctx, Frame{T: "rpc", SID: sid, Payload: resp})
				_ = r.send(ctx, Frame{T: "close", SID: sid, Msg: "done"})
			}(f.SID, f.Payload)
		case "close":
			r.sessMu.Lock()
			cancel, ok := r.sessions[f.SID]
			r.sessMu.Unlock()
			if ok {
				cancel()
			}
		case "ping":
			_ = r.send(ctx, Frame{T: "pong"})
		}
	}
}

func (r *RelayClient) send(ctx context.Context, f Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
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
