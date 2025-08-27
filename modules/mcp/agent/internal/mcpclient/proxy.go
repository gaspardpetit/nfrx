package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/gaspardpetit/nfrx/modules/common/logx"
	"github.com/gaspardpetit/nfrx/internal/mcpbridge"
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
	sessions map[string]*sessionState
}

type sessionState struct {
	conn    Connector
	handler func(mcp.JSONRPCNotification)

	mu      sync.Mutex
	pending map[string]chan json.RawMessage
	nextID  atomic.Int64
}

// NewProxy constructs a Proxy using the given websocket connection and factory.
func NewProxy(conn wsConn, factory ConnectorFactory) *Proxy {
	return &Proxy{conn: conn, factory: factory, sessions: map[string]*sessionState{}}
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
	sess, err := p.getSession(ctx, f.SessionID)
	if err != nil {
		return err
	}
	t := sess.conn.Transport()
	switch f.Type {
	case mcpbridge.TypeRequest:
		start := time.Now()
		var req transport.JSONRPCRequest
		if err := json.Unmarshal(f.Payload, &req); err != nil {
			return err
		}
		streamCount := 0
		orig := sess.handler
		t.SetNotificationHandler(func(n mcp.JSONRPCNotification) {
			streamCount++
			b, _ := json.Marshal(n)
			out := mcpbridge.Frame{Type: mcpbridge.TypeStreamEvent, ID: f.ID, SessionID: f.SessionID, Payload: b}
			ob, _ := json.Marshal(out)
			_ = p.conn.Write(context.Background(), websocket.MessageText, ob)
		})
		resp, err := t.SendRequest(ctx, req)
		t.SetNotificationHandler(orig)
		if err != nil {
			return err
		}
		b, _ := json.Marshal(resp)
		logx.Log.Info().Str("session", f.SessionID).Int("req_bytes", len(f.Payload)).Int("resp_bytes", len(b)).Int("stream_events", streamCount).Dur("duration", time.Since(start)).Msg("mcp request")
		out := mcpbridge.Frame{Type: mcpbridge.TypeResponse, ID: f.ID, SessionID: f.SessionID, Payload: b}
		ob, _ := json.Marshal(out)
		return p.conn.Write(ctx, websocket.MessageText, ob)
	case mcpbridge.TypeNotification:
		var n mcp.JSONRPCNotification
		if err := json.Unmarshal(f.Payload, &n); err != nil {
			return err
		}
		return t.SendNotification(ctx, n)
	case mcpbridge.TypeServerResponse:
		sess.mu.Lock()
		ch := sess.pending[f.ID]
		if ch != nil {
			delete(sess.pending, f.ID)
		}
		sess.mu.Unlock()
		if ch != nil {
			ch <- f.Payload
		}
	}
	return nil
}

func (p *Proxy) getSession(ctx context.Context, id string) (*sessionState, error) {
	p.mu.Lock()
	sess := p.sessions[id]
	p.mu.Unlock()
	if sess != nil {
		return sess, nil
	}
	if p.factory == nil {
		return nil, context.Canceled
	}
	c, err := p.factory(ctx, id)
	if err != nil {
		return nil, err
	}
	t := c.Transport()
	handler := func(n mcp.JSONRPCNotification) {
		b, _ := json.Marshal(n)
		f := mcpbridge.Frame{Type: mcpbridge.TypeNotification, SessionID: id, Payload: b}
		ob, _ := json.Marshal(f)
		_ = p.conn.Write(context.Background(), websocket.MessageText, ob)
	}
	t.SetNotificationHandler(handler)

	st := &sessionState{conn: c, handler: handler, pending: map[string]chan json.RawMessage{}}

	logx.Log.Info().Str("session", id).Str("transport", c.Protocol()).Msg("downstream connected")

	if bidir, ok := t.(transport.BidirectionalInterface); ok {
		bidir.SetRequestHandler(func(ctx context.Context, req transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
			b, _ := json.Marshal(req)
			idNum := st.nextID.Add(1)
			corrID := fmt.Sprintf("srv-%d", idNum)
			ch := make(chan json.RawMessage, 1)
			st.mu.Lock()
			st.pending[corrID] = ch
			st.mu.Unlock()
			frame := mcpbridge.Frame{Type: mcpbridge.TypeServerRequest, ID: corrID, SessionID: id, Payload: b}
			ob, _ := json.Marshal(frame)
			if err := p.conn.Write(context.Background(), websocket.MessageText, ob); err != nil {
				st.mu.Lock()
				delete(st.pending, corrID)
				st.mu.Unlock()
				return nil, err
			}
			select {
			case respBytes := <-ch:
				var resp transport.JSONRPCResponse
				if err := json.Unmarshal(respBytes, &resp); err != nil {
					return nil, err
				}
				return &resp, nil
			case <-ctx.Done():
				st.mu.Lock()
				delete(st.pending, corrID)
				st.mu.Unlock()
				return nil, ctx.Err()
			}
		})
	}

	p.mu.Lock()
	p.sessions[id] = st
	p.mu.Unlock()
	return st, nil
}
