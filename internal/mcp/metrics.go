package mcp

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gaspardpetit/nfrx/internal/logx"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartMetricsServer starts an HTTP server exposing Prometheus metrics on /metrics.
// It returns the address it is listening on.
func StartMetricsServer(ctx context.Context, addr string) (string, error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	actual := ln.Addr().String()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logx.Log.Error().Err(err).Str("addr", actual).Msg("metrics server error")
		}
	}()
	return actual, nil
}
