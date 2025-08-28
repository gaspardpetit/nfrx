package api

import (
	"net/http/httptest"
	"testing"
)

type fakeHealth struct{ ok bool }

func (f fakeHealth) Healthy() bool { return f.ok }

func TestGetHealthz(t *testing.T) {
	api := &API{Health: fakeHealth{ok: false}}
	rr := httptest.NewRecorder()
	api.GetHealthz(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != 503 {
		t.Fatalf("want 503 got %d", rr.Code)
	}
	api.Health = fakeHealth{ok: true}
	rr = httptest.NewRecorder()
	api.GetHealthz(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != 200 {
		t.Fatalf("want 200 got %d", rr.Code)
	}
}
