package api

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"github.com/gaspardpetit/nfrx/core/logx"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (lw *loggingResponseWriter) WriteHeader(status int) {
	lw.status = status
	lw.ResponseWriter.WriteHeader(status)
}

func (lw *loggingResponseWriter) Write(b []byte) (int, error) {
	if zerolog.GlobalLevel() <= zerolog.DebugLevel {
		logx.Log.Debug().Bytes("body", b).Msg("http response chunk")
	}
	return lw.ResponseWriter.Write(b)
}

func (lw *loggingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := lw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacker not supported")
}

func (lw *loggingResponseWriter) Flush() {
	if f, ok := lw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (lw *loggingResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := lw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func MiddlewareChain() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		chiMiddleware.RequestID,
		requestLogger,
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lvl := zerolog.GlobalLevel()
		lrw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		if lvl <= zerolog.DebugLevel {
			var body []byte
			if r.Body != nil {
				body, _ = io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			logx.Log.Debug().Str("method", r.Method).Str("url", r.URL.String()).Interface("headers", r.Header).Bytes("body", body).Msg("http request")
		}
		next.ServeHTTP(lrw, r)
		if lvl <= zerolog.DebugLevel {
			logx.Log.Debug().Str("url", r.URL.String()).Int("status", lrw.status).Interface("headers", lrw.Header()).Msg("http response")
		} else if lvl <= zerolog.InfoLevel {
			logx.Log.Info().Str("url", r.URL.String()).Int("status", lrw.status).Msg("http")
		}
	})
}

// APIKeyMiddleware checks the Authorization header for a matching API key.
func APIKeyMiddleware(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != apiKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				if _, err := w.Write([]byte(`{"error":"unauthorized"}`)); err != nil {
					logx.Log.Error().Err(err).Msg("write unauthorized")
				}
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
