package mcpbroker

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
	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/internal/logx"
	"github.com/gaspardpetit/nfrx/internal/mcp"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Relay tracks a connected MCP relay.
type Relay struct {
	conn     *websocket.Conn
	mu       sync.Mutex
	pending  map[string]chan mcp.Frame
	inflight int
	lastSeen time.Time
	methods  map[string]int
	sessions map[string]sessionInfo
	name     string
}

type sessionInfo struct {
	method string
	start  time.Time
}

// Registry stores active relays keyed by client ID.
type Registry struct {
	mu             sync.RWMutex
	relays         map[string]*Relay
	maxReqBytes    int64
	maxRespBytes   int64
	requestTimeout time.Duration
	heartbeat      time.Duration
	deadAfter      time.Duration
	maxConc        int
}

// NewRegistry constructs a Registry using environment variables for configuration.
func NewRegistry(timeout time.Duration) *Registry {
	maxReqBytes := int64(parseInt(getEnv("BROKER_MAX_REQ_BYTES", "10485760")))
	maxRespBytes := int64(parseInt(getEnv("BROKER_MAX_RESP_BYTES", "10485760")))
	heartbeat := time.Duration(parseInt(getEnv("BROKER_WS_HEARTBEAT_MS", "15000"))) * time.Millisecond
	deadAfter := time.Duration(parseInt(getEnv("BROKER_WS_DEAD_AFTER_MS", "45000"))) * time.Millisecond
	maxConc := parseInt(getEnv("BROKER_MAX_CONCURRENCY_PER_CLIENT", "16"))
	return &Registry{relays: map[string]*Relay{}, maxReqBytes: maxReqBytes, maxRespBytes: maxRespBytes, requestTimeout: timeout, heartbeat: heartbeat, deadAfter: deadAfter, maxConc: maxConc}
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
func (r *Registry) WSHandler(clientKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if serverstate.IsDraining() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
		c, err := websocket.Accept(w, req, nil)
		if err != nil {
			return
		}
		reqCtx := req.Context()
		_, data, err := c.Read(reqCtx)
		if err != nil {
			_ = c.Close(websocket.StatusPolicyViolation, "expected register")
			return
		}
		var reg struct {
			ID         string `json:"id"`
			ClientName string `json:"client_name"`
			ClientKey  string `json:"client_key"`
		}
		if err := json.Unmarshal(data, &reg); err != nil {
			_ = c.Close(websocket.StatusPolicyViolation, "invalid register")
			return
		}
		key := reg.ClientKey
		if clientKey == "" && key != "" {
			_ = c.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}
		if clientKey != "" && key != clientKey {
			_ = c.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}
		clientID := reg.ID
		if clientID == "" {
			clientID = uuid.NewString()
		}
		r.mu.Lock()
		if _, exists := r.relays[clientID]; exists {
			r.mu.Unlock()
			_ = c.Close(websocket.StatusPolicyViolation, "id in use")
			return
		}
		relay := &Relay{conn: c, pending: map[string]chan mcp.Frame{}, lastSeen: time.Now(), methods: map[string]int{}, sessions: map[string]sessionInfo{}, name: reg.ClientName}
		r.relays[clientID] = relay
		r.mu.Unlock()

		ack, _ := json.Marshal(map[string]string{"id": clientID})
		_ = c.Write(reqCtx, websocket.MessageText, ack)
		logx.Log.Info().Str("client_id", clientID).Str("client_name", reg.ClientName).Msg("mcp relay registered")

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
		var f mcp.Frame
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
			_ = relay.write(context.Background(), mcp.Frame{T: "ping"})
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
func (r *Registry) Snapshot() ctrlsrv.MCPState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	state := ctrlsrv.MCPState{}
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
		state.Clients = append(state.Clients, ctrlsrv.MCPClientSnapshot{
			ID:        id,
			Name:      rl.name,
			Status:    status,
			Inflight:  rl.inflight,
			Functions: funcs,
		})
		for sid, s := range rl.sessions {
			state.Sessions = append(state.Sessions, ctrlsrv.MCPSessionSnapshot{
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

func (rl *Relay) register(sid string) chan mcp.Frame {
	ch := make(chan mcp.Frame, 4)
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

func (rl *Relay) write(ctx context.Context, f mcp.Frame) error {
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
		if serverstate.IsDraining() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
		clientID := chi.URLParam(req, "id")
		reqID := uuid.NewString()
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
		auth := ""
		if h := req.Header.Get("Authorization"); h != "" {
			if strings.HasPrefix(strings.ToLower(h), "bearer ") {
				auth = strings.TrimSpace(h[7:])
			}
		}
		ctx, cancel := context.WithTimeout(req.Context(), r.requestTimeout)
		defer cancel()
		if err := relay.write(ctx, mcp.Frame{T: "open", SID: sid, ReqID: reqID, Hint: env.Method, Auth: auth}); err != nil {
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_PROVIDER_UNAVAILABLE").Msg("relay write failed")
			writeRPCError(w, env.ID, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "relay write failed", reqID)
			return
		}
		select {
		case f := <-ch:
			if f.T != "open.ok" {
				status := http.StatusServiceUnavailable
				code := "MCP_PROVIDER_UNAVAILABLE"
				if f.Code == "MCP_UNAUTHORIZED" {
					status = http.StatusUnauthorized
					code = f.Code
				}
				logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", code).Msg("open failed")
				writeRPCError(w, env.ID, status, code, "open failed", reqID)
				return
			}
		case <-ctx.Done():
			_ = relay.write(context.Background(), mcp.Frame{T: "close", SID: sid, Msg: "timeout"})
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_TIMEOUT").Msg("timeout waiting for open")
			writeRPCError(w, env.ID, http.StatusGatewayTimeout, "MCP_TIMEOUT", "timeout waiting for open", reqID)
			return
		}
		if err := relay.write(ctx, mcp.Frame{T: "rpc", SID: sid, Payload: raw}); err != nil {
			_ = relay.write(context.Background(), mcp.Frame{T: "close", SID: sid, Msg: "relay_write_failed"})
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_PROVIDER_UNAVAILABLE").Msg("relay write failed")
			writeRPCError(w, env.ID, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "relay write failed", reqID)
			return
		}
		var resp mcp.Frame
		select {
		case resp = <-ch:
			if len(resp.Payload) > int(r.maxRespBytes) {
				_ = relay.write(context.Background(), mcp.Frame{T: "close", SID: sid, Msg: "resp_too_large"})
				logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_LIMIT_EXCEEDED").Msg("response too large")
				writeRPCError(w, env.ID, http.StatusOK, "MCP_LIMIT_EXCEEDED", "response too large", reqID)
				return
			}
		case <-ctx.Done():
			_ = relay.write(context.Background(), mcp.Frame{T: "close", SID: sid, Msg: "timeout"})
			logx.Log.Warn().Str("component", "server.http").Str("client_id", clientID).Str("req_id", reqID).Str("error_code", "MCP_TIMEOUT").Msg("timeout waiting for response")
			writeRPCError(w, env.ID, http.StatusGatewayTimeout, "MCP_TIMEOUT", "timeout waiting for response", reqID)
			return
		}
		_ = relay.write(context.Background(), mcp.Frame{T: "close", SID: sid, Msg: "done"})
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
