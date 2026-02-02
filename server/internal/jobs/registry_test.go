package jobs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gaspardpetit/nfrx/server/internal/transfer"
)

func TestHandleStatusUpdateRejectsTerminalJobs(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	job := &Job{
		ID:        "job-1",
		Type:      "test",
		Status:    StatusCanceled,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	reg.mu.Lock()
	reg.jobs[job.ID] = job
	reg.mu.Unlock()

	router := chi.NewRouter()
	reg.RegisterWorkerRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/jobs/job-1/status", strings.NewReader(`{"state":"running"}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "invalid_state" {
		t.Fatalf("expected error invalid_state, got %v", resp["error"])
	}

	reg.mu.Lock()
	status := reg.jobs[job.ID].Status
	reg.mu.Unlock()
	if status != StatusCanceled {
		t.Fatalf("expected status to remain %q, got %q", StatusCanceled, status)
	}
}
