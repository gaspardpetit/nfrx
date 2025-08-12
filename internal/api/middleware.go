package api

import (
	"net/http"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/you/llamapool/internal/logx"
)

func middlewareChain() []func(http.Handler) http.Handler {
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
