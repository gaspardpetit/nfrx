package worker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/you/llamapool/internal/logx"
)

// StartStatusServer starts an HTTP server exposing /status and /version.
// It returns the address it is listening on.
func StartStatusServer(ctx context.Context, addr string) (string, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GetState())
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GetVersionInfo())
	})

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
