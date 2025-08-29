package adapters

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/gaspardpetit/nfrx/sdk/api/mcp"
    "github.com/gaspardpetit/nfrx/sdk/base/tunnel"
)

// MCPRegisterDecoder parses the initial register frame from an MCP relay.
func MCPRegisterDecoder(first []byte) (id, name, clientKey string, err error) {
    var reg struct{
        ID string `json:"id"`
        ClientName string `json:"client_name"`
        ClientKey string `json:"client_key"`
    }
    err = json.Unmarshal(first, &reg)
    if err != nil { return "", "", "", err }
    return reg.ID, reg.ClientName, reg.ClientKey, nil
}

// MCPReadLoop reads frames from the relay and routes them to pending sessions.
func MCPReadLoop(ctx context.Context, rl *tunnel.Relay) {
    // Send ack with assigned client ID for compatibility
    _ = rl.Conn.Write(ctx, 1, mustJSON(map[string]string{"id": rl.ID}))
    for {
        _, data, err := rl.Conn.Read(ctx)
        if err != nil { return }
        rl.Mu.Lock()
        rl.LastSeen = time.Now()
        rl.Mu.Unlock()
        var f mcp.Frame
        if json.Unmarshal(data, &f) != nil { continue }
        if f.T == "pong" { continue }
        if f.SID != "" {
            rl.Mu.Lock(); ch := rl.Pending[f.SID]; rl.Mu.Unlock()
            if ch != nil {
                select { case ch <- data: default: }
            }
        }
    }
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

// MCPAdapter implements tunnel.Adapter for JSON-RPC 2.0 over MCP frames.
type MCPAdapter struct{}

func (MCPAdapter) JobType() string { return "mcp.call" }

func (MCPAdapter) ValidateRequest(body []byte) (label string, id any, payload []byte, status int, errCode string, ok bool) {
    var env struct{
        JSONRPC string `json:"jsonrpc"`
        ID any `json:"id"`
        Method string `json:"method"`
    }
    if json.Unmarshal(body, &env) != nil || env.JSONRPC != "2.0" || env.ID == nil || env.Method == "" {
        return "", nil, nil, http.StatusOK, "MCP_SCHEMA_ERROR", false
    }
    return env.Method, env.ID, body, 0, "", true
}

func (MCPAdapter) WriteError(w http.ResponseWriter, id any, status int, errCode, msg, reqID string) {
    w.Header().Set("Content-Type", "application/json")
    if status == 0 { status = http.StatusBadGateway }
    w.WriteHeader(status)
    errObj := map[string]any{
        "jsonrpc": "2.0",
        "id": id,
        "error": map[string]any{
            "code": -32000,
            "message": msg,
            "data": map[string]any{ "mcp": errCode, "req_id": reqID },
        },
    }
    _ = json.NewEncoder(w).Encode(errObj)
}

func (MCPAdapter) WriteEmptyResult(w http.ResponseWriter, id any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    resp := map[string]any{"jsonrpc":"2.0","id": id, "result": map[string]any{}}
    _ = json.NewEncoder(w).Encode(resp)
}

func (MCPAdapter) Open(ctx context.Context, rl *tunnel.Relay, sid, reqID, label, auth string) (bool, string, int) {
    f := mcp.Frame{T:"open", SID: sid, ReqID: reqID, Hint: label, Auth: auth}
    b, _ := json.Marshal(f)
    rl.Mu.Lock(); err := rl.Conn.Write(ctx, 1, b); rl.Mu.Unlock()
    if err != nil { return false, "MCP_PROVIDER_UNAVAILABLE", http.StatusServiceUnavailable }
    return true, "", 0
}

func (MCPAdapter) Send(ctx context.Context, rl *tunnel.Relay, sid string, payload []byte) error {
    f := mcp.Frame{T:"rpc", SID: sid, Payload: payload}
    b, _ := json.Marshal(f)
    rl.Mu.Lock(); err := rl.Conn.Write(ctx, 1, b); rl.Mu.Unlock()
    return err
}

func (MCPAdapter) WaitOpen(ctx context.Context, ch <-chan []byte) (bool, string, int) {
    select {
    case data := <-ch:
        var f mcp.Frame
        if json.Unmarshal(data, &f) != nil { return false, "MCP_PROVIDER_UNAVAILABLE", http.StatusServiceUnavailable }
        if f.T != "open.ok" {
            code := "MCP_PROVIDER_UNAVAILABLE"; status := http.StatusServiceUnavailable
            if f.Code == "MCP_UNAUTHORIZED" { code = f.Code; status = http.StatusUnauthorized }
            return false, code, status
        }
        return true, "", 0
    case <-ctx.Done():
        return false, "MCP_TIMEOUT", http.StatusGatewayTimeout
    }
}

func (MCPAdapter) WaitResponse(ctx context.Context, ch <-chan []byte, maxRespBytes int64) ([]byte, bool, string, int) {
    select {
    case data := <-ch:
        var f mcp.Frame
        if json.Unmarshal(data, &f) != nil { return nil, false, "MCP_PROVIDER_UNAVAILABLE", http.StatusServiceUnavailable }
        if int64(len(f.Payload)) > maxRespBytes { return nil, false, "MCP_LIMIT_EXCEEDED", http.StatusOK }
        return f.Payload, true, "", 0
    case <-ctx.Done():
        return nil, false, "MCP_TIMEOUT", http.StatusGatewayTimeout
    }
}

func (MCPAdapter) Close(ctx context.Context, rl *tunnel.Relay, sid string, reason string) error {
    f := mcp.Frame{T:"close", SID: sid, Msg: reason}
    b, _ := json.Marshal(f)
    rl.Mu.Lock(); err := rl.Conn.Write(ctx, 1, b); rl.Mu.Unlock()
    return err
}
