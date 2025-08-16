package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// ErrBackpressure indicates the session queue is full.
var ErrBackpressure = errors.New("session backpressure")

// Bridge manages per-session WebSocket connections and forwards JSON-RPC payloads.
type Bridge struct {
	url         string
	maxInflight int

	mu       sync.Mutex
	sessions map[string]*session

	serverReqCh chan ServerRequest
}

// ServerRequest represents a request initiated by the downstream MCP server.
type ServerRequest struct {
	ID        string
	SessionID string
	Payload   json.RawMessage
}

// ServerRequests returns a channel of incoming server-initiated requests.
func (b *Bridge) ServerRequests() <-chan ServerRequest { return b.serverReqCh }

// ServerRespond sends a response payload back to the downstream server for the given correlation ID.
func (b *Bridge) ServerRespond(ctx context.Context, sessionID, id string, payload json.RawMessage) error {
	sess, err := b.getSession(ctx, sessionID)
	if err != nil {
		return err
	}
	frame := Frame{Type: TypeServerResponse, ID: id, SessionID: sessionID, Payload: payload}
	select {
	case sess.send <- frame:
		return nil
	default:
		return ErrBackpressure
	}
}

// ServerStream forwards a stream event for a server-initiated request.
func (b *Bridge) ServerStream(ctx context.Context, sessionID, id string, payload json.RawMessage) error {
	sess, err := b.getSession(ctx, sessionID)
	if err != nil {
		return err
	}
	frame := Frame{Type: TypeStreamEvent, ID: id, SessionID: sessionID, Payload: payload}
	select {
	case sess.send <- frame:
		return nil
	default:
		return ErrBackpressure
	}
}

// NewBridge constructs a new Bridge that dials the given WebSocket URL.
func NewBridge(url string, maxInflight int) *Bridge {
	if maxInflight <= 0 {
		maxInflight = 16
	}
	return &Bridge{url: url, maxInflight: maxInflight, sessions: map[string]*session{}, serverReqCh: make(chan ServerRequest, maxInflight)}
}

// Forward sends a JSON-RPC request payload over the bridge and waits for the response.
func (b *Bridge) Forward(ctx context.Context, sessionID string, payload json.RawMessage, jsonID json.RawMessage, stream func(json.RawMessage)) (json.RawMessage, error) {
	sess, err := b.getSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	corrID := sess.idMap.Alloc(jsonID)
	respCh := make(chan json.RawMessage, 1)
	if !sess.register(corrID, respCh, stream) {
		return nil, ErrBackpressure
	}
	frame := Frame{Type: TypeRequest, ID: corrID, SessionID: sessionID, Payload: payload}
	select {
	case sess.send <- frame:
	default:
		sess.unregister(corrID)
		return nil, ErrBackpressure
	}
	select {
	case resp, ok := <-respCh:
		sess.unregister(corrID)
		sess.idMap.Resolve(corrID)
		if !ok {
			return nil, ErrSessionClosed
		}
		return resp, nil
	case <-ctx.Done():
		sess.unregister(corrID)
		sess.idMap.Resolve(corrID)
		return nil, ctx.Err()
	}
}

// Close closes all active sessions.
func (b *Bridge) Close() {
	b.mu.Lock()
	sessions := b.sessions
	b.sessions = map[string]*session{}
	b.mu.Unlock()
	for _, s := range sessions {
		_ = s.conn.Close(websocket.StatusNormalClosure, "shutdown")
	}
	close(b.serverReqCh)
}

// ErrSessionClosed indicates the session was closed while waiting for a response.
var ErrSessionClosed = errors.New("session closed")

type session struct {
	id   string
	conn *websocket.Conn
	send chan Frame

	idMap *IDMapper

	mu          sync.Mutex
	pending     map[string]pendingReq
	maxInflight int

	cancel context.CancelFunc

	serverReqCh chan<- ServerRequest
}

type pendingReq struct {
	ch     chan json.RawMessage
	stream func(json.RawMessage)
}

func newSession(ctx context.Context, url, id string, maxInflight int, onClose func(), reqCh chan<- ServerRequest) (*session, error) {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &session{
		id:          id,
		conn:        conn,
		send:        make(chan Frame, maxInflight),
		idMap:       NewIDMapper(),
		pending:     map[string]pendingReq{},
		maxInflight: maxInflight,
		cancel:      cancel,
		serverReqCh: reqCh,
	}
	go s.readLoop(ctx, onClose)
	go s.writeLoop()
	go s.pingLoop(ctx)
	return s, nil
}

func (b *Bridge) getSession(ctx context.Context, id string) (*session, error) {
	b.mu.Lock()
	s := b.sessions[id]
	b.mu.Unlock()
	if s != nil {
		return s, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	// double check
	if s = b.sessions[id]; s != nil {
		return s, nil
	}
	sess, err := newSession(ctx, b.url, id, b.maxInflight, func() {
		b.mu.Lock()
		delete(b.sessions, id)
		b.mu.Unlock()
	}, b.serverReqCh)
	if err != nil {
		return nil, err
	}
	b.sessions[id] = sess
	return sess, nil
}

func (s *session) register(id string, ch chan json.RawMessage, stream func(json.RawMessage)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) >= s.maxInflight {
		return false
	}
	s.pending[id] = pendingReq{ch: ch, stream: stream}
	return true
}

func (s *session) unregister(id string) {
	s.mu.Lock()
	p := s.pending[id]
	delete(s.pending, id)
	s.mu.Unlock()
	if p.ch != nil {
		close(p.ch)
	}
}

func (s *session) readLoop(ctx context.Context, onClose func()) {
	defer func() {
		onClose()
		s.cancel()
		_ = s.conn.Close(websocket.StatusNormalClosure, "closing")
		s.mu.Lock()
		for id, p := range s.pending {
			if p.ch != nil {
				close(p.ch)
			}
			delete(s.pending, id)
		}
		s.mu.Unlock()
	}()
	for {
		readCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		_, data, err := s.conn.Read(readCtx)
		cancel()
		if err != nil {
			return
		}
		var f Frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		switch f.Type {
		case TypeResponse:
			s.mu.Lock()
			p := s.pending[f.ID]
			delete(s.pending, f.ID)
			s.mu.Unlock()
			if p.ch != nil {
				p.ch <- f.Payload
			}
		case TypeStreamEvent:
			s.mu.Lock()
			p := s.pending[f.ID]
			s.mu.Unlock()
			if p.stream != nil {
				p.stream(f.Payload)
			}
		case TypeServerRequest:
			if s.serverReqCh != nil {
				s.serverReqCh <- ServerRequest{ID: f.ID, SessionID: f.SessionID, Payload: f.Payload}
			}
		}
	}
}

func (s *session) writeLoop() {
	for f := range s.send {
		b, err := json.Marshal(f)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.conn.Write(ctx, websocket.MessageText, b)
		cancel()
	}
}

func (s *session) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.conn.Ping(ctx)
		case <-ctx.Done():
			return
		}
	}
}
