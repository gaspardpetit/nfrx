package api

import (
	"encoding/json"
	"net/http"
	"strings"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/you/llamapool/internal/logx"
)

func middlewareChain() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{
		chiMiddleware.RequestID,
		requestLogger,
	}
}

// APIKeyMiddleware enforces a shared API key for client requests.
func APIKeyMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
				return
			}
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != key {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := chiMiddleware.GetReqID(r.Context())
		logx.Log.Info().Str("request_id", reqID).Str("method", r.Method).Str("path", r.URL.Path).Msg("request")
		next.ServeHTTP(w, r)
	})
}
