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
	conn    *websocket.Conn
	mu      sync.Mutex
	pending map[string]chan Frame
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
	return &Registry{relays: map[string]*Relay{}, allowed: allowed, token: token, maxReqBytes: maxReqBytes, maxRespBytes: maxRespBytes, callTimeout: callTimeout}
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
		c, err := websocket.Accept(w, req, nil)
		if err != nil {
			return
		}
		relay := &Relay{conn: c, pending: map[string]chan Frame{}}
		r.mu.Lock()
		r.relays[clientID] = relay
		r.mu.Unlock()
		ctx := req.Context()
		go r.readPump(ctx, clientID, relay)
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
			continue
		}
		relay.mu.Lock()
		ch := relay.pending[f.SID]
		relay.mu.Unlock()
		if ch != nil {
			ch <- f
		}
	}
}

func (r *Registry) getRelay(clientID string) *Relay {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.relays[clientID]
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
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.conn.Write(ctx, websocket.MessageText, b)
}

// HTTPHandler handles host JSON-RPC requests.
func (r *Registry) HTTPHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		clientID := chi.URLParam(req, "client_id")
		if len(r.allowed) > 0 && !r.allowed[clientID] {
			writeRPCError(w, nil, http.StatusForbidden, "MCP_POLICY_DENIED", "client not allowed", "")
			return
		}
		relay := r.getRelay(clientID)
		if relay == nil {
			writeRPCError(w, nil, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "relay offline", "")
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
			writeRPCError(w, nil, http.StatusOK, "MCP_SCHEMA_ERROR", "invalid json-rpc", "")
			return
		}
		reqID := uuid.NewString()
		sid := uuid.NewString()
		ch := relay.register(sid)
		defer relay.unregister(sid)
		ctx, cancel := context.WithTimeout(req.Context(), r.callTimeout)
		defer cancel()
		if err := relay.write(ctx, Frame{T: "open", SID: sid, ReqID: reqID, Hint: env.Method}); err != nil {
			writeRPCError(w, env.ID, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "relay write failed", reqID)
			return
		}
		select {
		case f := <-ch:
			if f.T != "open.ok" {
				writeRPCError(w, env.ID, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "open failed", reqID)
				return
			}
		case <-ctx.Done():
			writeRPCError(w, env.ID, http.StatusGatewayTimeout, "MCP_TIMEOUT", "timeout waiting for open", reqID)
			return
		}
		if err := relay.write(ctx, Frame{T: "rpc", SID: sid, Payload: raw}); err != nil {
			writeRPCError(w, env.ID, http.StatusServiceUnavailable, "MCP_PROVIDER_UNAVAILABLE", "relay write failed", reqID)
			return
		}
		var resp Frame
		select {
		case resp = <-ch:
			if len(resp.Payload) > int(r.maxRespBytes) {
				writeRPCError(w, env.ID, http.StatusOK, "MCP_LIMIT_EXCEEDED", "response too large", reqID)
				return
			}
		case <-ctx.Done():
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
