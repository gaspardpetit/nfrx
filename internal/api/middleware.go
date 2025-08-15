package api

import (
	"net/http"
	"strings"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/you/llamapool/internal/logx"
)

func MiddlewareChain() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		chiMiddleware.RequestID,
		requestLogger,
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := chiMiddleware.GetReqID(r.Context())
		logx.Log.Info().Str("request_id", reqID).Str("method", r.Method).Str("path", r.URL.Path).Msg("request")
		next.ServeHTTP(w, r)
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
