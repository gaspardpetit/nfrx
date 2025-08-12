package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestRequestIDMiddleware(t *testing.T) {
	chain := middlewareChain()
	var captured string
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = chiMiddleware.GetReqID(r.Context())
	})
	for i := len(chain) - 1; i >= 0; i-- {
		h = chain[i](h)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)
	if captured == "" {
		t.Fatalf("missing request id")
	}
}
