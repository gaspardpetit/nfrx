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

func TestAPIKeyMiddleware(t *testing.T) {
	h := APIKeyMiddleware("sekret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// missing header
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	// wrong key
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer nope")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	// correct key
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer sekret")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	// api disabled
	h = APIKeyMiddleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
