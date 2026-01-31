package tunnel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	basemetrics "github.com/gaspardpetit/nfrx/sdk/base/metrics"
	"github.com/google/uuid"
)

// Config defines generic tunnel tunables.
type Config struct {
	Heartbeat               time.Duration
	DeadAfter               time.Duration
	MaxConcurrencyPerClient int
}

// Relay is a generic connected client tunnel.
type Relay struct {
	Conn     *websocket.Conn
	Mu       sync.Mutex
	Pending  map[string]chan []byte
	Inflight int
	LastSeen time.Time
	Name     string
	ID       string
	Sessions map[string]SessionInfo
	Methods  map[string]int
}

type SessionInfo struct {
	Method string
	Start  time.Time
}

// Registry manages connected relays.
type Registry struct {
	mu       sync.RWMutex
	relays   map[string]*Relay
	cfg      Config
	draining func() bool
}

// New creates a new generic tunnel registry.
func New(cfg Config, drainingFn func() bool) *Registry {
	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 15 * time.Second
	}
	if cfg.DeadAfter == 0 {
		cfg.DeadAfter = 45 * time.Second
	}
	return &Registry{relays: make(map[string]*Relay), cfg: cfg, draining: drainingFn}
}

// RegisterAdapter decodes the first WS message and returns id,name,clientKey.
type RegisterAdapter func(first []byte) (id, name, clientKey string, err error)

// WSHandler accepts tunnel connections using a RegisterAdapter.
// ReadLoop should invoke onClose exactly once before returning.
type ReadLoop func(ctx context.Context, rl *Relay, onClose func())

// WSHandler accepts tunnel connections. Authorization is granted when either the
// provided register key matches expectKey or when X-User-Roles contains any role
// listed in allowedRoles.
func (r *Registry) WSHandler(expectKey string, decode RegisterAdapter, reader ReadLoop, allowedRoles ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if r.draining != nil && r.draining() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
		c, err := websocket.Accept(w, req, nil)
		if err != nil {
			return
		}
		// Disable default 32KiB read limit to support large messages
		c.SetReadLimit(-1)
		// Use background context for long-lived WS loops; request context may be canceled when handler returns.
		ctx := context.Background()
		_, data, err := c.Read(ctx)
		if err != nil {
			_ = c.Close(websocket.StatusPolicyViolation, "expected register")
			return
		}
		id, name, _, err := decode(data)
		if err != nil {
			_ = c.Close(websocket.StatusPolicyViolation, "invalid register")
			return
		}
		authorized := expectKey == "" || hasAnyAllowedRole(req.Header.Get("X-User-Roles"), allowedRoles) || checkBearer(req.Header.Get("Authorization"), expectKey)
		if !authorized {
			_ = c.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}

		// Assign a server-side ID when none is provided and reject duplicates
		if id == "" {
			id = uuid.NewString()
		}
		r.mu.Lock()
		if _, exists := r.relays[id]; exists {
			r.mu.Unlock()
			_ = c.Close(websocket.StatusPolicyViolation, "id in use")
			return
		}
		rl := &Relay{Conn: c, Pending: map[string]chan []byte{}, Sessions: map[string]SessionInfo{}, Methods: map[string]int{}, LastSeen: time.Now(), Name: name, ID: id}
		r.relays[id] = rl
		r.mu.Unlock()
		go r.heartbeatLoop(ctx, id, rl)
		if reader != nil {
			go reader(ctx, rl, func() { r.mu.Lock(); delete(r.relays, id); r.mu.Unlock() })
		}
	}
}

func hasAnyAllowedRole(header string, allowed []string) bool {
	if header == "" || len(allowed) == 0 {
		return false
	}
	m := map[string]struct{}{}
	for _, r := range allowed {
		rr := strings.TrimSpace(r)
		if rr != "" {
			m[rr] = struct{}{}
		}
	}
	for _, it := range strings.Split(header, ",") {
		if _, ok := m[strings.TrimSpace(it)]; ok {
			return true
		}
	}
	return false
}

func checkBearer(authHeader, expected string) bool {
	if expected == "" {
		return false
	}
	ah := strings.TrimSpace(authHeader)
	if ah == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(ah), "bearer ") {
		tok := strings.TrimSpace(ah[7:])
		return tok == expected
	}
	return false
}

func (r *Registry) heartbeatLoop(ctx context.Context, id string, rl *Relay) {
	ticker := time.NewTicker(r.cfg.Heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.Mu.Lock()
			last := rl.LastSeen
			rl.Mu.Unlock()
			if time.Since(last) > r.cfg.DeadAfter {
				_ = rl.Conn.Close(websocket.StatusNormalClosure, "dead")
				return
			}
			rl.Mu.Lock()
			_ = rl.Conn.Write(ctx, websocket.MessageText, mustJSON(map[string]string{"type": "ping"}))
			rl.Mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// Register a session channel for responses addressed by sid.
func (rl *Relay) Register(sid string) chan []byte {
	ch := make(chan []byte, 4)
	rl.Mu.Lock()
	rl.Pending[sid] = ch
	rl.Mu.Unlock()
	return ch
}
func (rl *Relay) Unregister(sid string) { rl.Mu.Lock(); delete(rl.Pending, sid); rl.Mu.Unlock() }

// Snapshot captures a generic view of connected relays and sessions.
type ClientSnapshot struct {
	ID, Name, Status string
	Inflight         int
	Methods          map[string]int
}
type SessionSnapshot struct {
	ID, ClientID, Method string
	StartedAt            time.Time
	DurationMs           uint64
}
type State struct {
	Clients  []ClientSnapshot
	Sessions []SessionSnapshot
}

func (r *Registry) Snapshot() State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	st := State{}
	for id, rl := range r.relays {
		rl.Mu.Lock()
		mcopy := make(map[string]int, len(rl.Methods))
		for k, v := range rl.Methods {
			mcopy[k] = v
		}
		status := "idle"
		if rl.Inflight > 0 {
			status = "active"
		}
		st.Clients = append(st.Clients, ClientSnapshot{ID: id, Name: rl.Name, Status: status, Inflight: rl.Inflight, Methods: mcopy})
		for sid, s := range rl.Sessions {
			st.Sessions = append(st.Sessions, SessionSnapshot{ID: sid, ClientID: id, Method: s.Method, StartedAt: s.Start, DurationMs: uint64(time.Since(s.Start).Milliseconds())})
		}
		rl.Mu.Unlock()
	}
	return st
}

// Adapter defines protocol hooks for the HTTP relay path.
type Adapter interface {
	// JobType returns the job type string for metrics (e.g., "mcp.call").
	JobType() string
	// ValidateRequest parses the incoming HTTP body and returns the label (e.g., method), request id to echo,
	// normalized payload to send over the relay, or an error response.
	ValidateRequest(body []byte) (label string, id any, payload []byte, status int, errCode string, ok bool)
	WriteError(w http.ResponseWriter, id any, status int, errCode, msg, reqID string)
	WriteEmptyResult(w http.ResponseWriter, id any)
	// Session lifecycle
	Open(ctx context.Context, rl *Relay, sid, reqID, label, auth string) (ok bool, errCode string, status int)
	Send(ctx context.Context, rl *Relay, sid string, payload []byte) error
	WaitOpen(ctx context.Context, ch <-chan []byte) (ok bool, errCode string, status int)
	WaitResponse(ctx context.Context, ch <-chan []byte, maxRespBytes int64) (payload []byte, ok bool, errCode string, status int)
	Close(ctx context.Context, rl *Relay, sid string, reason string) error
}

// HTTPHandler returns a generic tunnel HTTP relay handler.
// getID extracts the client ID from the request (e.g., via chi URL param).
func (r *Registry) HTTPHandler(ext string, getID func(*http.Request) string, adapter Adapter, requestTimeout time.Duration, maxReqBytes, maxRespBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if r.draining != nil && r.draining() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
		clientID := getID(req)
		r.mu.RLock()
		relay := r.relays[clientID]
		r.mu.RUnlock()
		reqID := uuid.NewString()
		if relay == nil {
			adapter.WriteError(w, nil, http.StatusServiceUnavailable, "PROVIDER_UNAVAILABLE", "relay offline", reqID)
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, maxReqBytes)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		label, id, payload, status, errCode, ok := adapter.ValidateRequest(body)
		jobType := adapter.JobType()
		// Record generic request metric
		basemetrics.RecordRequest(ext, "tunnel", jobType, label)
		if !ok {
			adapter.WriteError(w, id, status, errCode, "invalid request", reqID)
			basemetrics.RecordComplete(ext, "tunnel", jobType, label, errCode, false, 0)
			return
		}
		// Simple bearer token extraction
		auth := ""
		if h := req.Header.Get("Authorization"); h != "" {
			if strings.HasPrefix(strings.ToLower(h), "bearer ") {
				auth = strings.TrimSpace(h[7:])
			}
		}
		// Concurrency check and session register
		relay.Mu.Lock()
		if r.cfg.MaxConcurrencyPerClient > 0 && relay.Inflight >= r.cfg.MaxConcurrencyPerClient {
			relay.Mu.Unlock()
			adapter.WriteError(w, id, http.StatusTooManyRequests, "LIMIT_EXCEEDED", "too many concurrent calls", reqID)
			basemetrics.RecordComplete(ext, "tunnel", jobType, label, "LIMIT_EXCEEDED", false, 0)
			return
		}
		relay.Inflight++
		if relay.Methods != nil {
			relay.Methods[label]++
		}
		sid := uuid.NewString()
		relay.Sessions[sid] = SessionInfo{Method: label, Start: time.Now()}
		relay.Mu.Unlock()
		ch := relay.Register(sid)
		start := time.Now()
		defer func() {
			relay.Unregister(sid)
			relay.Mu.Lock()
			relay.Inflight--
			if relay.Methods != nil {
				relay.Methods[label]--
			}
			delete(relay.Sessions, sid)
			relay.Mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(req.Context(), requestTimeout)
		defer cancel()
		if ok, code, st := adapter.Open(ctx, relay, sid, reqID, label, auth); !ok {
			adapter.WriteError(w, id, st, code, "open failed", reqID)
			basemetrics.RecordComplete(ext, "tunnel", jobType, label, code, false, time.Since(start))
			return
		}
		if ok, code, st := adapter.WaitOpen(ctx, ch); !ok {
			adapter.WriteError(w, id, st, code, "open failed", reqID)
			basemetrics.RecordComplete(ext, "tunnel", jobType, label, code, false, time.Since(start))
			return
		}
		// Mark started after successful open
		basemetrics.RecordStart(ext, "tunnel", jobType, label)
		if err := adapter.Send(ctx, relay, sid, payload); err != nil {
			_ = adapter.Close(context.Background(), relay, sid, "relay_write_failed")
			adapter.WriteError(w, id, http.StatusServiceUnavailable, "PROVIDER_UNAVAILABLE", "relay write failed", reqID)
			basemetrics.RecordComplete(ext, "tunnel", jobType, label, "PROVIDER_UNAVAILABLE", false, time.Since(start))
			return
		}
		respBody, ok, code, st := adapter.WaitResponse(ctx, ch, maxRespBytes)
		if !ok {
			_ = adapter.Close(context.Background(), relay, sid, "timeout")
			adapter.WriteError(w, id, st, code, "timeout waiting for response", reqID)
			basemetrics.RecordComplete(ext, "tunnel", jobType, label, code, false, time.Since(start))
			return
		}
		_ = adapter.Close(context.Background(), relay, sid, "done")
		if len(respBody) == 0 {
			adapter.WriteEmptyResult(w, id)
			basemetrics.RecordComplete(ext, "tunnel", jobType, label, "", true, time.Since(start))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
		basemetrics.RecordComplete(ext, "tunnel", jobType, label, "", true, time.Since(start))
	}
}
