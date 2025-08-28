package worker

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

	"github.com/gaspardpetit/nfrx/modules/common/logx"
)

// StartStatusServer starts an HTTP server exposing status, version, and control endpoints.
// The token for control endpoints is stored alongside the config file.
// It returns the address it is listening on.
func StartStatusServer(ctx context.Context, addr, configFile string, drainTimeout time.Duration, shutdown func()) (string, error) {
	tokenPath := filepath.Join(filepath.Dir(defaultConfigPath(configFile)), "worker.token")
	token, err := loadOrCreateToken(tokenPath)
	if err != nil {
		return "", err
	}

	var (
		mu    sync.Mutex
		timer *time.Timer
	)

	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if r.Header.Get("X-Auth-Token") != token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GetState())
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GetVersionInfo())
	})
	mux.HandleFunc("/control/drain", auth(func(w http.ResponseWriter, r *http.Request) {
		StartDrain()
		mu.Lock()
		if timer != nil {
			timer.Stop()
		}
		if drainTimeout > 0 {
			timer = time.AfterFunc(drainTimeout, func() {
				if IsDraining() {
					SetState("terminating")
					shutdown()
				}
			})
		} else {
			timer = nil
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/control/undrain", auth(func(w http.ResponseWriter, r *http.Request) {
		StopDrain()
		mu.Lock()
		if timer != nil {
			timer.Stop()
			timer = nil
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	mux.HandleFunc("/control/shutdown", auth(func(w http.ResponseWriter, r *http.Request) {
		SetState("terminating")
		shutdown()
		w.WriteHeader(http.StatusOK)
	}))

	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	actual := ln.Addr().String()
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logx.Log.Error().Err(err).Str("addr", actual).Msg("status server error")
		}
	}()
	return actual, nil
}

func defaultConfigPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return "worker.yaml"
	}
	return p
}

func loadOrCreateToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		tok := strings.TrimSpace(string(b))
		if tok != "" {
			return tok, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(tok), 0o600); err != nil {
		return "", err
	}
	return tok, nil
}
