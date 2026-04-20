package jobs

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestClaimJobFiltersByWorkerAffinity(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	now := time.Now()

	reg.mu.Lock()
	reg.jobs["job-any"] = &Job{ID: "job-any", Type: "test", Status: StatusQueued, CreatedAt: now, UpdatedAt: now}
	reg.jobs["job-worker"] = &Job{ID: "job-worker", Type: "test", Status: StatusQueued, WorkerID: "worker-a", CreatedAt: now, UpdatedAt: now}
	reg.jobs["job-group"] = &Job{ID: "job-group", Type: "test", Status: StatusQueued, WorkerGroup: "group-a", CreatedAt: now, UpdatedAt: now}
	reg.jobs["job-both"] = &Job{ID: "job-both", Type: "test", Status: StatusQueued, WorkerID: "worker-b", WorkerGroup: "group-a", CreatedAt: now, UpdatedAt: now}
	reg.queue = []string{"job-worker", "job-group", "job-both", "job-any"}
	reg.mu.Unlock()

	job := reg.claimNext([]string{"test"}, "worker-a", "group-z")
	if job == nil || job.ID != "job-worker" {
		t.Fatalf("expected worker-targeted job, got %#v", job)
	}
	if job.ClaimedWorkerID != "worker-a" || job.ClaimedWorkerGroup != "group-z" {
		t.Fatalf("expected claimed worker fields to be recorded, got %#v", job)
	}

	job = reg.claimNext([]string{"test"}, "worker-z", "group-a")
	if job == nil || job.ID != "job-group" {
		t.Fatalf("expected group-targeted job, got %#v", job)
	}

	job = reg.claimNext([]string{"test"}, "worker-b", "group-a")
	if job == nil || job.ID != "job-both" {
		t.Fatalf("expected exact worker/group job, got %#v", job)
	}

	job = reg.claimNext([]string{"test"}, "worker-c", "group-c")
	if job == nil || job.ID != "job-any" {
		t.Fatalf("expected untargeted job, got %#v", job)
	}
}

func TestHandleCreateAndClaimAffinityFields(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)

	router := chi.NewRouter()
	reg.RegisterRoutes(router)

	createReq := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{"type":"test","worker_id":"worker-a","worker_group":"group-a"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d", createResp.Code, http.StatusOK)
	}

	var created struct {
		JobID string `json:"job_id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	claimReq := httptest.NewRequest(http.MethodPost, "/jobs/claim", strings.NewReader(`{"worker_id":"worker-a","worker_group":"group-a","max_wait_seconds":0}`))
	claimReq.Header.Set("Content-Type", "application/json")
	claimResp := httptest.NewRecorder()
	router.ServeHTTP(claimResp, claimReq)
	if claimResp.Code != http.StatusOK {
		t.Fatalf("claim status = %d, want %d", claimResp.Code, http.StatusOK)
	}

	var claimed map[string]any
	if err := json.NewDecoder(claimResp.Body).Decode(&claimed); err != nil {
		t.Fatalf("decode claim: %v", err)
	}
	if claimed["worker_id"] != "worker-a" {
		t.Fatalf("expected worker_id in claim response, got %v", claimed["worker_id"])
	}
	if claimed["worker_group"] != "group-a" {
		t.Fatalf("expected worker_group in claim response, got %v", claimed["worker_group"])
	}
	if claimed["claimed_worker_id"] != "worker-a" {
		t.Fatalf("expected claimed_worker_id in claim response, got %v", claimed["claimed_worker_id"])
	}
	if claimed["claimed_worker_group"] != "group-a" {
		t.Fatalf("expected claimed_worker_group in claim response, got %v", claimed["claimed_worker_group"])
	}

	getReq := httptest.NewRequest(http.MethodGet, "/jobs/"+created.JobID, nil)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getResp.Code, http.StatusOK)
	}

	var view map[string]any
	if err := json.NewDecoder(getResp.Body).Decode(&view); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if view["worker_id"] != "worker-a" || view["worker_group"] != "group-a" {
		t.Fatalf("expected requested affinity in view, got %#v", view)
	}
	if view["claimed_worker_id"] != "worker-a" || view["claimed_worker_group"] != "group-a" {
		t.Fatalf("expected claimed affinity in view, got %#v", view)
	}
}

func TestClaimStreamEmitsCompatibleJobs(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	now := time.Now()

	reg.mu.Lock()
	reg.jobs["job-a"] = &Job{ID: "job-a", Type: "test", Status: StatusQueued, WorkerGroup: "group-a", CreatedAt: now, UpdatedAt: now}
	reg.jobs["job-b"] = &Job{ID: "job-b", Type: "other", Status: StatusQueued, WorkerGroup: "group-b", CreatedAt: now, UpdatedAt: now}
	reg.queue = []string{"job-b", "job-a"}
	reg.mu.Unlock()

	router := chi.NewRouter()
	reg.RegisterWorkerRoutes(router)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/jobs/stream?types=test&worker_id=worker-17&worker_group=group-a")
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("stream content-type = %q, want %q", got, "text/event-stream")
	}

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read event line: %v", err)
	}
	if strings.TrimSpace(line) != "event: job" {
		t.Fatalf("first line = %q, want event: job", strings.TrimSpace(line))
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read data line: %v", err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("data line = %q, want data prefix", line)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(line), "data: ")), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["job_id"] != "job-a" {
		t.Fatalf("job_id = %v, want job-a", payload["job_id"])
	}
	if payload["claimed_worker_id"] != "worker-17" {
		t.Fatalf("claimed_worker_id = %v, want worker-17", payload["claimed_worker_id"])
	}
	if payload["claimed_worker_group"] != "group-a" {
		t.Fatalf("claimed_worker_group = %v, want group-a", payload["claimed_worker_group"])
	}

	reg.mu.Lock()
	status := reg.jobs["job-a"].Status
	reg.mu.Unlock()
	if status != StatusClaimed {
		t.Fatalf("job status = %q, want %q", status, StatusClaimed)
	}
}

func TestClaimStreamStopsClaimingAfterDisconnect(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	now := time.Now()

	reg.mu.Lock()
	reg.jobs["job-a"] = &Job{ID: "job-a", Type: "test", Status: StatusQueued, WorkerGroup: "group-a", CreatedAt: now, UpdatedAt: now}
	reg.jobs["job-b"] = &Job{ID: "job-b", Type: "test", Status: StatusQueued, WorkerGroup: "group-a", CreatedAt: now, UpdatedAt: now}
	reg.queue = []string{"job-a"}
	reg.mu.Unlock()

	router := chi.NewRouter()
	reg.RegisterWorkerRoutes(router)
	srv := httptest.NewServer(router)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/jobs/stream?types=test&worker_id=worker-17&worker_group=group-a", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	reader := bufio.NewReader(resp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read event line: %v", err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("read data line: %v", err)
	}
	cancel()
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}

	reg.mu.Lock()
	reg.queue = append(reg.queue, "job-b")
	reg.mu.Unlock()
	reg.signal()

	deadline := time.Now().Add(2 * time.Second)
	for {
		reg.mu.Lock()
		statusA := reg.jobs["job-a"].Status
		statusB := reg.jobs["job-b"].Status
		queueLen := len(reg.queue)
		reg.mu.Unlock()
		if statusA == StatusClaimed {
			if statusB == StatusQueued && queueLen == 1 {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("unexpected post-disconnect state: job-a=%q job-b=%q queue=%d", statusA, statusB, queueLen)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStateSnapshotTracksRecentWorkers(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	now := time.Now()

	reg.mu.Lock()
	reg.jobs["job-1"] = &Job{ID: "job-1", Type: "test", Status: StatusQueued, WorkerGroup: "group-a", CreatedAt: now, UpdatedAt: now}
	reg.queue = []string{"job-1"}
	reg.mu.Unlock()

	reg.recordWorkerSeen("worker-a", "group-a", "poll", []string{"test"}, "")
	reg.addWorkerStream("worker-b", "group-b", []string{"test"})
	state := reg.StateSnapshot()

	if state.Summary.ActiveWorkers != 2 {
		t.Fatalf("active_workers = %d, want 2", state.Summary.ActiveWorkers)
	}
	if state.Summary.StreamingWorkers != 1 {
		t.Fatalf("streaming_workers = %d, want 1", state.Summary.StreamingWorkers)
	}
	if state.Summary.QueuedJobs != 1 {
		t.Fatalf("queued_jobs = %d, want 1", state.Summary.QueuedJobs)
	}
	if len(state.Workers) != 2 {
		t.Fatalf("workers len = %d, want 2", len(state.Workers))
	}
	if len(state.Jobs) != 1 || state.Jobs[0].ID != "job-1" {
		t.Fatalf("unexpected jobs snapshot: %#v", state.Jobs)
	}
}

func TestStateSnapshotPrunesStaleWorkers(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	stale := time.Now().Add(-workerRetention - time.Minute)

	reg.mu.Lock()
	reg.workers[workerKey("worker-old", "group-a")] = &WorkerActivity{
		Key:         workerKey("worker-old", "group-a"),
		WorkerID:    "worker-old",
		WorkerGroup: "group-a",
		LastSeenAt:  stale,
	}
	reg.workers[workerKey("worker-live", "group-b")] = &WorkerActivity{
		Key:         workerKey("worker-live", "group-b"),
		WorkerID:    "worker-live",
		WorkerGroup: "group-b",
		LastSeenAt:  time.Now(),
	}
	reg.mu.Unlock()

	state := reg.StateSnapshot()
	if len(state.Workers) != 1 {
		t.Fatalf("workers len = %d, want 1", len(state.Workers))
	}
	if state.Workers[0].WorkerID != "worker-live" {
		t.Fatalf("remaining worker = %q, want worker-live", state.Workers[0].WorkerID)
	}
}

func TestHandlePayloadRequestStoresProperties(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	job := &Job{
		ID:        "job-payload",
		Type:      "test",
		Status:    StatusClaimed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	reg.mu.Lock()
	reg.jobs[job.ID] = job
	reg.mu.Unlock()

	ch := reg.subscribe(job.ID)
	defer reg.unsubscribe(job.ID, ch)

	router := chi.NewRouter()
	reg.RegisterRoutes(router)

	body := `{"key":"payload","properties":{"protocol":"demo-v1","options":{"mode":"header-body","note":"opaque to nfrx"}}}`
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+job.ID+"/payload", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("payload status = %d, want %d", resp.Code, http.StatusOK)
	}

	var payloadResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		t.Fatalf("decode payload response: %v", err)
	}
	want := map[string]any{
		"protocol": "demo-v1",
		"options": map[string]any{
			"mode": "header-body",
			"note": "opaque to nfrx",
		},
	}
	if !reflect.DeepEqual(payloadResp["properties"], want) {
		t.Fatalf("payload response properties = %#v, want %#v", payloadResp["properties"], want)
	}

	ev := expectEventType(t, ch, "payload")
	info, ok := ev.Data.(*TransferInfo)
	if !ok {
		t.Fatalf("payload event data type = %T, want *TransferInfo", ev.Data)
	}
	if !reflect.DeepEqual(info.Properties, want) {
		t.Fatalf("payload event properties = %#v, want %#v", info.Properties, want)
	}
	_ = expectEventType(t, ch, "status")

	getReq := httptest.NewRequest(http.MethodGet, "/jobs/"+job.ID, nil)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getResp.Code, http.StatusOK)
	}

	var view JobView
	if err := json.NewDecoder(getResp.Body).Decode(&view); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	got := view.Payloads["payload"]
	if got == nil || !reflect.DeepEqual(got.Properties, want) {
		t.Fatalf("job payload properties = %#v, want %#v", got, want)
	}
}

func TestHandleResultRequestStoresProperties(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	job := &Job{
		ID:        "job-result",
		Type:      "test",
		Status:    StatusRunning,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	reg.mu.Lock()
	reg.jobs[job.ID] = job
	reg.mu.Unlock()

	ch := reg.subscribe(job.ID)
	defer reg.unsubscribe(job.ID, ch)

	router := chi.NewRouter()
	reg.RegisterRoutes(router)

	body := `{"key":"result","properties":{"protocol":"demo-v1","options":{"mode":"header-body","note":"opaque to nfrx"}}}`
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+job.ID+"/result", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("result status = %d, want %d", resp.Code, http.StatusOK)
	}

	var resultResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&resultResp); err != nil {
		t.Fatalf("decode result response: %v", err)
	}
	want := map[string]any{
		"protocol": "demo-v1",
		"options": map[string]any{
			"mode": "header-body",
			"note": "opaque to nfrx",
		},
	}
	if !reflect.DeepEqual(resultResp["properties"], want) {
		t.Fatalf("result response properties = %#v, want %#v", resultResp["properties"], want)
	}

	ev := expectEventType(t, ch, "result")
	info, ok := ev.Data.(*TransferInfo)
	if !ok {
		t.Fatalf("result event data type = %T, want *TransferInfo", ev.Data)
	}
	if !reflect.DeepEqual(info.Properties, want) {
		t.Fatalf("result event properties = %#v, want %#v", info.Properties, want)
	}
	_ = expectEventType(t, ch, "status")

	getReq := httptest.NewRequest(http.MethodGet, "/jobs/"+job.ID, nil)
	getResp := httptest.NewRecorder()
	router.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getResp.Code, http.StatusOK)
	}

	var view JobView
	if err := json.NewDecoder(getResp.Body).Decode(&view); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	got := view.Results["result"]
	if got == nil || !reflect.DeepEqual(got.Properties, want) {
		t.Fatalf("job result properties = %#v, want %#v", got, want)
	}
}

func TestHandlePayloadRequestOmitsPropertiesWhenUnset(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	job := &Job{
		ID:        "job-payload-empty",
		Type:      "test",
		Status:    StatusClaimed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	reg.mu.Lock()
	reg.jobs[job.ID] = job
	reg.mu.Unlock()

	router := chi.NewRouter()
	reg.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/jobs/"+job.ID+"/payload", strings.NewReader(`{"key":"payload"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("payload status = %d, want %d", resp.Code, http.StatusOK)
	}

	var payloadResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		t.Fatalf("decode payload response: %v", err)
	}
	if _, ok := payloadResp["properties"]; ok {
		t.Fatalf("expected properties to be omitted, got %#v", payloadResp["properties"])
	}
}

func TestHandleCancelJobIsIdempotentForTerminalJobs(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	job := &Job{
		ID:        "job-completed",
		Type:      "test",
		Status:    StatusCompleted,
		CreatedAt: time.Now().Add(-time.Minute),
		UpdatedAt: time.Now().Add(-30 * time.Second),
	}
	reg.mu.Lock()
	reg.jobs[job.ID] = job
	reg.mu.Unlock()

	router := chi.NewRouter()
	reg.RegisterClientRoutes(router)

	req := httptest.NewRequest(http.MethodPost, "/jobs/"+job.ID+"/cancel", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want %d", resp.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	if body["status"] != StatusCompleted {
		t.Fatalf("cancel response status = %v, want %q", body["status"], StatusCompleted)
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	if reg.jobs[job.ID].Status != StatusCompleted {
		t.Fatalf("job status = %q, want %q", reg.jobs[job.ID].Status, StatusCompleted)
	}
}

func TestStateSnapshotTracksLifetimeOutcomesAndDurations(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	base := time.Now().Add(-time.Hour)

	jobCompleted := &Job{ID: "job-completed", Type: "test", Status: StatusQueued, CreatedAt: base, UpdatedAt: base}
	jobFailed := &Job{ID: "job-failed", Type: "test", Status: StatusQueued, CreatedAt: base.Add(10 * time.Second), UpdatedAt: base.Add(10 * time.Second)}
	jobCanceled := &Job{ID: "job-canceled", Type: "test", Status: StatusQueued, CreatedAt: base.Add(20 * time.Second), UpdatedAt: base.Add(20 * time.Second)}

	reg.mu.Lock()
	reg.jobs[jobCompleted.ID] = jobCompleted
	reg.jobs[jobFailed.ID] = jobFailed
	reg.jobs[jobCanceled.ID] = jobCanceled
	reg.setJobStatusLocked(jobCompleted, StatusClaimed, base.Add(5*time.Second))
	reg.setJobStatusLocked(jobCompleted, StatusCompleted, base.Add(20*time.Second))
	reg.setJobStatusLocked(jobFailed, StatusClaimed, base.Add(20*time.Second))
	reg.setJobStatusLocked(jobFailed, StatusRunning, base.Add(25*time.Second))
	reg.setJobStatusLocked(jobFailed, StatusFailed, base.Add(40*time.Second))
	reg.setJobStatusLocked(jobCanceled, StatusCanceled, base.Add(65*time.Second))
	reg.mu.Unlock()

	state := reg.StateSnapshot()
	if state.Summary.CompletedJobs != 1 {
		t.Fatalf("completed_jobs = %d, want 1", state.Summary.CompletedJobs)
	}
	if state.Summary.FailedJobs != 1 {
		t.Fatalf("failed_jobs = %d, want 1", state.Summary.FailedJobs)
	}
	if state.Summary.CanceledJobs != 1 {
		t.Fatalf("canceled_jobs = %d, want 1", state.Summary.CanceledJobs)
	}
	if state.Summary.AvgQueueWaitMS != 20000 {
		t.Fatalf("avg_queue_wait_ms = %d, want 20000", state.Summary.AvgQueueWaitMS)
	}
	if state.Summary.AvgServiceMS != 17500 {
		t.Fatalf("avg_service_ms = %d, want 17500", state.Summary.AvgServiceMS)
	}
	if state.Summary.AvgEndToEndMS != 31666 {
		t.Fatalf("avg_end_to_end_ms = %d, want 31666", state.Summary.AvgEndToEndMS)
	}
	if state.Summary.LastCompletedAt != base.Add(20*time.Second).UTC().Format(time.RFC3339) {
		t.Fatalf("last_completed_at = %q", state.Summary.LastCompletedAt)
	}
	if state.Summary.LastFailedAt != base.Add(40*time.Second).UTC().Format(time.RFC3339) {
		t.Fatalf("last_failed_at = %q", state.Summary.LastFailedAt)
	}
	if state.Summary.LastCanceledAt != base.Add(65*time.Second).UTC().Format(time.RFC3339) {
		t.Fatalf("last_canceled_at = %q", state.Summary.LastCanceledAt)
	}
}

func TestStateSnapshotDoesNotDoubleCountTerminalTransitions(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	base := time.Now().Add(-time.Minute)
	job := &Job{ID: "job-1", Type: "test", Status: StatusQueued, CreatedAt: base, UpdatedAt: base}

	reg.mu.Lock()
	reg.jobs[job.ID] = job
	reg.setJobStatusLocked(job, StatusClaimed, base.Add(5*time.Second))
	reg.setJobStatusLocked(job, StatusRunning, base.Add(10*time.Second))
	reg.setJobStatusLocked(job, StatusCompleted, base.Add(20*time.Second))
	reg.setJobStatusLocked(job, StatusCompleted, base.Add(25*time.Second))
	reg.mu.Unlock()

	state := reg.StateSnapshot()
	if state.Summary.CompletedJobs != 1 {
		t.Fatalf("completed_jobs = %d, want 1", state.Summary.CompletedJobs)
	}
	if state.Summary.FailedJobs != 0 || state.Summary.CanceledJobs != 0 {
		t.Fatalf("unexpected terminal counts: %+v", state.Summary)
	}
	if state.Summary.AvgQueueWaitMS != 5000 {
		t.Fatalf("avg_queue_wait_ms = %d, want 5000", state.Summary.AvgQueueWaitMS)
	}
	if state.Summary.AvgServiceMS != 15000 {
		t.Fatalf("avg_service_ms = %d, want 15000", state.Summary.AvgServiceMS)
	}
}

func TestStateSnapshotTracksOldestQueueAndInflightAge(t *testing.T) {
	reg := NewRegistry(transfer.NewRegistry(0), 0, 0)
	now := time.Now()

	reg.mu.Lock()
	reg.jobs["job-queued"] = &Job{
		ID:        "job-queued",
		Type:      "test",
		Status:    StatusQueued,
		CreatedAt: now.Add(-2 * time.Minute),
		UpdatedAt: now.Add(-2 * time.Minute),
	}
	reg.jobs["job-running"] = &Job{
		ID:        "job-running",
		Type:      "test",
		Status:    StatusRunning,
		CreatedAt: now.Add(-3 * time.Minute),
		ClaimedAt: now.Add(-90 * time.Second),
		UpdatedAt: now.Add(-30 * time.Second),
	}
	reg.queue = []string{"job-queued"}
	reg.mu.Unlock()

	state := reg.StateSnapshot()
	if state.Summary.OldestQueuedSeconds < 119 || state.Summary.OldestQueuedSeconds > 121 {
		t.Fatalf("oldest_queued_seconds = %d, want about 120", state.Summary.OldestQueuedSeconds)
	}
	if state.Summary.OldestInflightSeconds < 89 || state.Summary.OldestInflightSeconds > 91 {
		t.Fatalf("oldest_inflight_seconds = %d, want about 90", state.Summary.OldestInflightSeconds)
	}
}

func expectEventType(t *testing.T, ch chan Event, typ string) Event {
	t.Helper()
	select {
	case ev := <-ch:
		if ev.Type != typ {
			t.Fatalf("event type = %q, want %q", ev.Type, typ)
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for event %q", typ)
		return Event{}
	}
}
