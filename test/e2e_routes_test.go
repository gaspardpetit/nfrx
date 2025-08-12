package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/server"
)

func TestRoutes(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{WorkerKey: "secret", WSPath: "/workers/connect", RequestTimeout: 5 * time.Second}
	handler := server.New(reg, sched, cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/tags")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/tags: %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = http.Get(srv.URL + "/tags")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for /tags")
	}
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz: %v %d", err, resp.StatusCode)
	}
	var v struct {
		Status string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&v)
	resp.Body.Close()
	if v.Status != "ok" {
		t.Fatalf("bad health body")
	}
}
