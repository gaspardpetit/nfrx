package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
	"github.com/gaspardpetit/llamapool/internal/logx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Frame represents a broker <-> relay frame.
type Frame struct {
	T       string          `json:"t"`
	SID     string          `json:"sid,omitempty"`
	ReqID   string          `json:"req_id,omitempty"`
	Hint    string          `json:"hint,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Code    string          `json:"code,omitempty"`
	Msg     string          `json:"msg,omitempty"`
}

// Relay tracks a connected MCP relay.
type Relay struct {
	conn     *websocket.Conn
	mu       sync.Mutex
	pending  map[string]chan Frame
	inflight int
	lastSeen time.Time
	methods  map[string]int
	sessions map[string]sessionInfo
}

type sessionInfo struct {
	method string
	start  time.Time
}

// Registry stores active relays keyed by client ID.
type Registry struct {
	mu           sync.RWMutex
	relays       map[string]*Relay
	allowed      map[string]bool
	token        string
	maxReqBytes  int64
	maxRespBytes int64
	callTimeout  time.Duration
	heartbeat    time.Duration
	deadAfter    time.Duration
	maxConc      int
}

// NewRegistry constructs a Registry using environment variables for configuration.
func NewRegistry() *Registry {
	allowed := map[string]bool{}
	for _, c := range strings.Split(getEnv("BROKER_ACCEPTED_CLIENTS", ""), ",") {
		if c != "" {
			allowed[strings.TrimSpace(c)] = true
		}
	}
	token := getEnv("BROKER_RELAY_TOKEN", "")
	maxReqBytes := int64(parseInt(getEnv("BROKER_MAX_REQ_BYTES", "10485760")))
	maxRespBytes := int64(parseInt(getEnv("BROKER_MAX_RESP_BYTES", "10485760")))
	callTimeout := time.Duration(parseInt(getEnv("BROKER_CALL_TIMEOUT_MS", "30000"))) * time.Millisecond
	heartbeat := time.Duration(parseInt(getEnv("BROKER_WS_HEARTBEAT_MS", "15000"))) * time.Millisecond
	deadAfter := time.Duration(parseInt(getEnv("BROKER_WS_DEAD_AFTER_MS", "45000"))) * time.Millisecond
	maxConc := parseInt(getEnv("BROKER_MAX_CONCURRENCY_PER_CLIENT", "16"))
	return &Registry{relays: map[string]*Relay{}, allowed: allowed, token: token, maxReqBytes: maxReqBytes, maxRespBytes: maxRespBytes, callTimeout: callTimeout, heartbeat: heartbeat, deadAfter: deadAfter, maxConc: maxConc}
}

func parseInt(v string) int {
	n, _ := strconv.Atoi(v)
	return n
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// WSHandler handles relay websocket connections.
func (r *Registry) WSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if r.token != "" {
			auth := req.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != r.token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		clientID := req.Header.Get("X-Client-Id")
		if clientID == "" {
			http.Error(w, "missing client id", http.StatusBadRequest)
			return
		}
		if len(r.allowed) > 0 && !r.allowed[clientID] {
			http.Error(w, "client not allowed", http.StatusForbidden)
			return
		}
		c, err := websocket.Accept(w, req, nil)
		if err != nil {
			return
		}
		relay := &Relay{conn: c, pending: map[string]chan Frame{}, lastSeen: time.Now(), methods: map[string]int{}, sessions: map[string]sessionInfo{}}
		r.mu.Lock()
		r.relays[clientID] = relay
		r.mu.Unlock()
		ctx := context.Background()
		go r.readPump(ctx, clientID, relay)
		go r.pingLoop(ctx, clientID, relay)
	}
}

func (r *Registry) readPump(ctx context.Context, clientID string, relay *Relay) {
	defer func() {
		_ = relay.conn.Close(websocket.StatusNormalClosure, "closing")
		r.mu.Lock()
		delete(r.relays, clientID)
		r.mu.Unlock()
	}()
	for {
		_, data, err := relay.conn.Read(ctx)
		if err != nil {
			return
		}
		var f Frame
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		if f.T == "pong" {
			relay.mu.Lock()
			relay.lastSeen = time.Now()
			relay.mu.Unlock()
			continue
		}
		relay.mu.Lock()
		relay.lastSeen = time.Now()
		relay.mu.Unlock()
		relay.mu.Lock()
		ch := relay.pending[f.SID]
		relay.mu.Unlock()
		if ch != nil {
			ch <- f
		}
	}
}

func (r *Registry) pingLoop(ctx context.Context, clientID string, relay *Relay) {
	ticker := time.NewTicker(r.heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			relay.mu.Lock()
			last := relay.lastSeen
			relay.mu.Unlock()
			if time.Since(last) > r.deadAfter {
				_ = relay.conn.Close(websocket.StatusNormalClosure, "dead")
				return
			}
			_ = relay.write(context.Background(), Frame{T: "ping"})
		case <-ctx.Done():
			return
		}
	}
}

func (r *Registry) getRelay(clientID string) *Relay {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.relays[clientID]
}

// Snapshot returns a snapshot of connected relays and active sessions.
func (r *Registry) Snapshot() ctrl.MCPState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	state := ctrl.MCPState{}
	for id, rl := range r.relays {
		rl.mu.Lock()
		funcs := make(map[string]int, len(rl.methods))
		for k, v := range rl.methods {
			funcs[k] = v
		}
		status := "idle"
		if rl.inflight > 0 {
			status = "active"
		}
		state.Clients = append(state.Clients, ctrl.MCPClientSnapshot{
			ID:        id,
			Status:    status,
			Inflight:  rl.inflight,
			Functions: funcs,
		})
		for sid, s := range rl.sessions {
			state.Sessions = append(state.Sessions, ctrl.MCPSessionSnapshot{
				ID:         sid,
				ClientID:   id,
				Method:     s.method,
				StartedAt:  s.start,
				DurationMs: uint64(time.Since(s.start).Milliseconds()),
			})
		}
		rl.mu.Unlock()
	}
	return state
}

func (rl *Relay) register(sid string) chan Frame {
	ch := make(chan Frame, 4)
	rl.mu.Lock()
	rl.pending[sid] = ch
	rl.mu.Unlock()
	return ch
}

func (rl *Relay) unregister(sid string) {
	rl.mu.Lock()
	delete(rl.pending, sid)
	rl.mu.Unlock()
}

func (rl *Relay) write(ctx context.Context, f Frame) error {
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.conn.Write(ctx, websocket.MessageText, b)
}

// HTTPHandler handles host JSON-RPC requests.
func (r *Registry) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		clientID := chi.URLParam(req, "client_id")
		reqID := uuid.NewString()
		if len(r.allowed) > 0 && !r.allowed[clientID] {
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_POLICY_DENIED").Msg("client not allowed")
			writeRPCError(w, nil, http.StatusForbidden, "MCP_POLICY_DENIED", "client not allowed", reqID)
			return
		}
		relay := r.getRelay(clientID)
		if relay == nil {
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_PROVIDER_UNAVAILABLE").Msg("relay offline")
			writeRPCError(w, nil, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "relay offline", reqID)
			return
		}
		req.Body = http.MaxBytesReader(w, req.Body, r.maxReqBytes)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var raw json.RawMessage = body
		var env struct {
			JSONRPC string `json:"jsonrpc"`
			ID      any    `json:"id"`
			Method  string `json:"method"`
		}
		if json.Unmarshal(body, &env) != nil || env.JSONRPC != "2.0" || env.ID == nil || env.Method == "" {
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_SCHEMA_ERROR").Msg("invalid json-rpc")
			writeRPCError(w, nil, http.StatusOK, "MCP_SCHEMA_ERROR", "invalid json-rpc", reqID)
			return
		}
		if env.Method == "cancel" {
			writeJSONRPCMethodNotFound(w, env.ID, reqID)
			return
		}
		relay.mu.Lock()
		if r.maxConc > 0 && relay.inflight >= r.maxConc {
			relay.mu.Unlock()
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_LIMIT_EXCEEDED").Msg("too many concurrent calls")
			writeRPCError(w, env.ID, http.StatusTooManyRequests, "MCP_LIMIT_EXCEEDED", "too many concurrent calls", reqID)
			return
		}
		relay.inflight++
		relay.methods[env.Method]++
		sid := uuid.NewString()
		relay.sessions[sid] = sessionInfo{method: env.Method, start: time.Now()}
		relay.mu.Unlock()
		ch := relay.register(sid)
		defer func() {
			relay.unregister(sid)
			relay.mu.Lock()
			relay.inflight--
			relay.methods[env.Method]--
			delete(relay.sessions, sid)
			relay.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(req.Context(), r.callTimeout)
		defer cancel()
		if err := relay.write(ctx, Frame{T: "open", SID: sid, ReqID: reqID, Hint: env.Method}); err != nil {
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_PROVIDER_UNAVAILABLE").Msg("relay write failed")
			writeRPCError(w, env.ID, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "relay write failed", reqID)
			return
		}
		select {
		case f := <-ch:
			if f.T != "open.ok" {
				logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_PROVIDER_UNAVAILABLE").Msg("open failed")
				writeRPCError(w, env.ID, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "open failed", reqID)
				return
			}
		case <-ctx.Done():
			_ = relay.write(context.Background(), Frame{T: "close", SID: sid, Msg: "timeout"})
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_TIMEOUT").Msg("timeout waiting for open")
			writeRPCError(w, env.ID, http.StatusGatewayTimeout, "MCP_TIMEOUT", "timeout waiting for open", reqID)
			return
		}
		if err := relay.write(ctx, Frame{T: "rpc", SID: sid, Payload: raw}); err != nil {
			_ = relay.write(context.Background(), Frame{T: "close", SID: sid, Msg: "relay_write_failed"})
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_PROVIDER_UNAVAILABLE").Msg("relay write failed")
			writeRPCError(w, env.ID, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "relay write failed", reqID)
			return
		}
		var resp Frame
		select {
		case resp = <-ch:
			if len(resp.Payload) > int(r.maxRespBytes) {
				_ = relay.write(context.Background(), Frame{T: "close", SID: sid, Msg: "resp_too_large"})
				logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_LIMIT_EXCEEDED").Msg("response too large")
				writeRPCError(w, env.ID, http.StatusOK, "MCP_LIMIT_EXCEEDED", "response too large", reqID)
				return
			}
		case <-ctx.Done():
			_ = relay.write(context.Background(), Frame{T: "close", SID: sid, Msg: "timeout"})
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_TIMEOUT").Msg("timeout waiting for response")
			writeRPCError(w, env.ID, http.StatusGatewayTimeout, "MCP_TIMEOUT", "timeout waiting for response", reqID)
			return
		}
		_ = relay.write(context.Background(), Frame{T: "close", SID: sid, Msg: "done"})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if len(resp.Payload) > 0 {
			_, _ = w.Write(resp.Payload)
		} else {
			_, _ = w.Write([]byte("{}"))
		}
		logx.Log.Info().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("method", env.Method).Msg("mcp request complete")
	}
}

func writeRPCError(w http.ResponseWriter, id any, status int, mcpCode, msg, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	errObj := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32000,
			"message": msg,
			"data": map[string]any{
				"mcp":    mcpCode,
				"req_id": reqID,
			},
		},
	}
	_ = json.NewEncoder(w).Encode(errObj)
}

func writeJSONRPCMethodNotFound(w http.ResponseWriter, id any, reqID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	errObj := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    -32601,
			"message": "Method not found",
			"data": map[string]any{
				"mcp":    "MCP_METHOD_NOT_FOUND",
				"req_id": reqID,
			},
		},
	}
	_ = json.NewEncoder(w).Encode(errObj)
}
