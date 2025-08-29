package drain

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "net"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "github.com/gaspardpetit/nfrx/sdk/base/agent"
)

// StartControlServer exposes /status, /version, and control endpoints using a token file.
// The token is stored at tokenPath; if empty, control endpoints are disabled.
// The statusFn and versionFn must be non-nil; drain/shutdown is optional.
func StartControlServer(ctx context.Context, addr, tokenPath string, drainTimeout time.Duration, statusFn func() any, versionFn func() any, shutdown func()) (string, error) {
    mux := http.NewServeMux()
    mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(statusFn())
    })
    mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(versionFn())
    })
    if strings.TrimSpace(tokenPath) != "" {
        token, err := loadOrCreateToken(tokenPath)
        if err != nil { return "", err }
        var (
            mu    sync.Mutex
            timer *time.Timer
        )
        auth := func(h http.HandlerFunc) http.HandlerFunc {
            return func(w http.ResponseWriter, r *http.Request) {
                if r.Method != http.MethodPost { w.WriteHeader(http.StatusMethodNotAllowed); return }
                host, _, _ := net.SplitHostPort(r.RemoteAddr)
                if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() { w.WriteHeader(http.StatusForbidden); return }
                if r.Header.Get("X-Auth-Token") != token { w.WriteHeader(http.StatusUnauthorized); return }
                h(w, r)
            }
        }
        mux.HandleFunc("/control/drain", auth(func(w http.ResponseWriter, r *http.Request) {
            Start()
            mu.Lock()
            if timer != nil { timer.Stop() }
            if drainTimeout > 0 {
                timer = time.AfterFunc(drainTimeout, func() {
                    if IsDraining() && shutdown != nil { shutdown() }
                })
            } else {
                timer = nil
            }
            mu.Unlock()
            w.WriteHeader(http.StatusOK)
        }))
        mux.HandleFunc("/control/undrain", auth(func(w http.ResponseWriter, r *http.Request) {
            Stop()
            mu.Lock(); if timer != nil { timer.Stop(); timer = nil } ; mu.Unlock()
            w.WriteHeader(http.StatusOK)
        }))
        mux.HandleFunc("/control/shutdown", auth(func(w http.ResponseWriter, r *http.Request) {
            if shutdown != nil { shutdown() }
            w.WriteHeader(http.StatusOK)
        }))
    }
    return agent.ServeUntilContext(ctx, addr, mux)
}

func loadOrCreateToken(path string) (string, error) {
    b, err := os.ReadFile(path)
    if err == nil {
        tok := strings.TrimSpace(string(b))
        if tok != "" { return tok, nil }
    }
    buf := make([]byte, 32)
    if _, err := rand.Read(buf); err != nil { return "", err }
    tok := hex.EncodeToString(buf)
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return "", err }
    if err := os.WriteFile(path, []byte(tok), 0o600); err != nil { return "", err }
    return tok, nil
}

