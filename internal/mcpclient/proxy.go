package mcpclient

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/coder/websocket"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/gaspardpetit/llamapool/internal/mcpbridge"
)

// wsConn abstracts a minimal websocket connection for testing.
type wsConn interface {
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Write(ctx context.Context, typ websocket.MessageType, data []byte) error
}

// ConnectorFactory creates a Connector for a session.
type ConnectorFactory func(ctx context.Context, sessionID string) (Connector, error)

// Proxy bridges websocket frames to a third-party MCP server via mcp-go transports.
type Proxy struct {
	conn     wsConn
	factory  ConnectorFactory
	mu       sync.Mutex
	sessions map[string]Connector
}

// NewProxy constructs a Proxy using the given websocket connection and factory.
func NewProxy(conn wsConn, factory ConnectorFactory) *Proxy {
	return &Proxy{conn: conn, factory: factory, sessions: map[string]Connector{}}
}

// Run processes frames until the connection errors.
func (p *Proxy) Run(ctx context.Context) error {
	for {
		_, data, err := p.conn.Read(ctx)
		if err != nil {
			return err
		}
		var f mcpbridge.Frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		if err := p.handleFrame(ctx, f); err != nil {
			return err
		}
	}
}

func (p *Proxy) handleFrame(ctx context.Context, f mcpbridge.Frame) error {
	conn, err := p.getSession(ctx, f.SessionID)
	if err != nil {
		return err
	}
	t := conn.Transport()
	switch f.Type {
	case mcpbridge.TypeRequest:
		var req transport.JSONRPCRequest
		if err := json.Unmarshal(f.Payload, &req); err != nil {
			return err
		}
		resp, err := t.SendRequest(ctx, req)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(resp)
		out := mcpbridge.Frame{Type: mcpbridge.TypeResponse, ID: f.ID, SessionID: f.SessionID, Payload: b}
		ob, _ := json.Marshal(out)
		return p.conn.Write(ctx, websocket.MessageText, ob)
	case mcpbridge.TypeNotification:
		var n mcp.JSONRPCNotification
		if err := json.Unmarshal(f.Payload, &n); err != nil {
			return err
		}
		return t.SendNotification(ctx, n)
	}
	return nil
}

func (p *Proxy) getSession(ctx context.Context, id string) (Connector, error) {
	p.mu.Lock()
	conn := p.sessions[id]
	p.mu.Unlock()
	if conn != nil {
		return conn, nil
	}
	if p.factory == nil {
		return nil, context.Canceled
	}
	c, err := p.factory(ctx, id)
	if err != nil {
		return nil, err
	}
	t := c.Transport()
	t.SetNotificationHandler(func(n mcp.JSONRPCNotification) {
		b, _ := json.Marshal(n)
		f := mcpbridge.Frame{Type: mcpbridge.TypeNotification, SessionID: id, Payload: b}
		ob, _ := json.Marshal(f)
		_ = p.conn.Write(context.Background(), websocket.MessageText, ob)
	})
	p.mu.Lock()
	p.sessions[id] = c
	p.mu.Unlock()
	return c, nil
}
