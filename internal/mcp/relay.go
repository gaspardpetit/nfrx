package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/coder/websocket"
)

// RelayClient is a minimal MCP relay.
type RelayClient struct {
	conn        *websocket.Conn
	providerURL string
}

// NewRelayClient creates a new relay client.
func NewRelayClient(conn *websocket.Conn, providerURL string) *RelayClient {
	return &RelayClient{conn: conn, providerURL: providerURL}
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
			_ = r.send(ctx, Frame{T: "open.ok", SID: f.SID})
		case "rpc":
			resp, err := r.callProvider(ctx, f.Payload)
			if err != nil {
				resp = []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32000,"message":"provider error"}}`)
			}
			_ = r.send(ctx, Frame{T: "rpc", SID: f.SID, Payload: resp})
			_ = r.send(ctx, Frame{T: "close", SID: f.SID, Msg: "done"})
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
	return r.conn.Write(ctx, websocket.MessageText, b)
}

func (r *RelayClient) callProvider(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.providerURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}
