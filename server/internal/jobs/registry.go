package jobs

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gaspardpetit/nfrx/core/logx"
	"github.com/gaspardpetit/nfrx/server/internal/transfer"
)

const (
	StatusQueued          = "queued"
	StatusClaimed         = "claimed"
	StatusAwaitingPayload = "awaiting_payload"
	StatusRunning         = "running"
	StatusAwaitingResult  = "awaiting_result"
	StatusCompleted       = "completed"
	StatusFailed          = "failed"
	StatusCanceled        = "canceled"

	workerActiveWindow = time.Minute
	workerRetention    = 10 * time.Minute
	maxStateJobs       = 12
)

type Registry struct {
	mu            sync.Mutex
	jobs          map[string]*Job
	queue         []string
	notify        chan struct{}
	subs          map[string]map[chan Event]struct{}
	transfer      *transfer.Registry
	sseCloseDelay time.Duration
	clientTTL     time.Duration
	clientTimers  map[string]*time.Timer
	workers       map[string]*WorkerActivity
}

type Job struct {
	ID                 string
	Type               string
	Status             string
	Metadata           map[string]any
	WorkerID           string
	WorkerGroup        string
	ClaimedWorkerID    string
	ClaimedWorkerGroup string
	Progress           map[string]any
	Error              *JobError
	Payloads           map[string]*TransferInfo
	Results            map[string]*TransferInfo
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type WorkerActivity struct {
	Key           string
	WorkerID      string
	WorkerGroup   string
	LastSeenAt    time.Time
	LastClaimAt   time.Time
	LastClaimMode string
	LastJobID     string
	Types         []string
	StreamCount   int
}

type TransferInfo struct {
	ChannelID string `json:"channel_id"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
	Key       string `json:"key,omitempty"`
}

type Event struct {
	Type string
	Data any
}

type CreateJobRequest struct {
	Type        string         `json:"type"`
	Priority    int            `json:"priority,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	WorkerID    string         `json:"worker_id,omitempty"`
	WorkerGroup string         `json:"worker_group,omitempty"`
}

type ClaimRequest struct {
	Types          []string `json:"types,omitempty"`
	MaxWaitSeconds int      `json:"max_wait_seconds,omitempty"`
	WorkerID       string   `json:"worker_id,omitempty"`
	WorkerGroup    string   `json:"worker_group,omitempty"`
}

type StatusUpdateRequest struct {
	State    string         `json:"state"`
	Progress map[string]any `json:"progress,omitempty"`
	Error    *JobError      `json:"error,omitempty"`
}

type TransferRequest struct {
	Key *string `json:"key,omitempty"`
}

type JobView struct {
	ID                 string                   `json:"id"`
	Type               string                   `json:"type"`
	Status             string                   `json:"status"`
	Metadata           map[string]any           `json:"metadata,omitempty"`
	WorkerID           string                   `json:"worker_id,omitempty"`
	WorkerGroup        string                   `json:"worker_group,omitempty"`
	ClaimedWorkerID    string                   `json:"claimed_worker_id,omitempty"`
	ClaimedWorkerGroup string                   `json:"claimed_worker_group,omitempty"`
	Progress           map[string]any           `json:"progress,omitempty"`
	Error              *JobError                `json:"error,omitempty"`
	Payloads           map[string]*TransferInfo `json:"payloads,omitempty"`
	Results            map[string]*TransferInfo `json:"results,omitempty"`
	QueuePosition      int                      `json:"queue_position,omitempty"`
	CreatedAt          string                   `json:"created_at"`
	UpdatedAt          string                   `json:"updated_at"`
}

type WorkerActivityView struct {
	Key           string   `json:"key"`
	WorkerID      string   `json:"worker_id,omitempty"`
	WorkerGroup   string   `json:"worker_group,omitempty"`
	LastSeenAt    string   `json:"last_seen_at"`
	LastClaimAt   string   `json:"last_claim_at,omitempty"`
	LastClaimMode string   `json:"last_claim_mode,omitempty"`
	LastJobID     string   `json:"last_job_id,omitempty"`
	Types         []string `json:"types,omitempty"`
	StreamCount   int      `json:"stream_count,omitempty"`
	Active        bool     `json:"active"`
}

type JobsSummary struct {
	TotalJobs         int `json:"total_jobs"`
	QueuedJobs        int `json:"queued_jobs"`
	ClaimedJobs       int `json:"claimed_jobs"`
	RunningJobs       int `json:"running_jobs"`
	AwaitingTransfers int `json:"awaiting_transfers"`
	TerminalJobs      int `json:"terminal_jobs"`
	ActiveWorkers     int `json:"active_workers"`
	RecentWorkers     int `json:"recent_workers"`
	StreamingWorkers  int `json:"streaming_workers"`
}

type StateView struct {
	Summary JobsSummary          `json:"summary"`
	Workers []WorkerActivityView `json:"workers"`
	Jobs    []JobView            `json:"jobs"`
}

func NewRegistry(tr *transfer.Registry, sseCloseDelay time.Duration, clientTTL time.Duration) *Registry {
	return &Registry{
		jobs:          make(map[string]*Job),
		queue:         []string{},
		notify:        make(chan struct{}, 1),
		subs:          make(map[string]map[chan Event]struct{}),
		transfer:      tr,
		sseCloseDelay: sseCloseDelay,
		clientTTL:     clientTTL,
		clientTimers:  make(map[string]*time.Timer),
		workers:       make(map[string]*WorkerActivity),
	}
}

func (r *Registry) RegisterRoutes(router chi.Router) {
	r.RegisterClientRoutes(router)
	r.RegisterWorkerRoutes(router)
}

func (r *Registry) RegisterClientRoutes(router chi.Router) {
	router.Post("/jobs", r.HandleCreateJob)
	router.Get("/jobs/{job_id}", r.HandleGetJob)
	router.Get("/jobs/{job_id}/events", r.HandleJobEvents)
	router.Post("/jobs/{job_id}/cancel", r.HandleCancelJob)
}

func (r *Registry) RegisterWorkerRoutes(router chi.Router) {
	router.Post("/jobs/claim", r.HandleClaimJob)
	router.Get("/jobs/stream", r.HandleClaimStream)
	router.Post("/jobs/{job_id}/payload", r.HandlePayloadRequest)
	router.Post("/jobs/{job_id}/result", r.HandleResultRequest)
	router.Post("/jobs/{job_id}/status", r.HandleStatusUpdate)
}

func (r *Registry) HandleCreateJob(w http.ResponseWriter, req *http.Request) {
	var body CreateJobRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if strings.TrimSpace(body.Type) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing_type"})
		return
	}
	job := &Job{
		ID:          uuid.NewString(),
		Type:        body.Type,
		Status:      StatusQueued,
		Metadata:    body.Metadata,
		WorkerID:    strings.TrimSpace(body.WorkerID),
		WorkerGroup: strings.TrimSpace(body.WorkerGroup),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	r.mu.Lock()
	r.jobs[job.ID] = job
	r.queue = append(r.queue, job.ID)
	view := r.viewLocked(job)
	r.mu.Unlock()
	r.publish(job.ID, Event{Type: "status", Data: view})
	r.startClientTimerIfIdle(job.ID)
	r.signal()
	writeJSON(w, http.StatusOK, map[string]any{"job_id": job.ID, "status": job.Status})
}

func (r *Registry) HandleGetJob(w http.ResponseWriter, req *http.Request) {
	view, pos, ok := r.snapshot(chi.URLParam(req, "job_id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	view.QueuePosition = pos
	writeJSON(w, http.StatusOK, view)
}

func (r *Registry) HandleJobEvents(w http.ResponseWriter, req *http.Request) {
	jobID := chi.URLParam(req, "job_id")
	view, _, ok := r.snapshot(jobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch := r.subscribe(jobID)
	defer r.onClientDisconnect(jobID)
	defer r.unsubscribe(jobID, ch)
	r.onClientConnect(jobID)

	// send initial status
	r.writeEvent(w, Event{Type: "status", Data: view})
	flusher.Flush()

	ctx := req.Context()
	var closeTimer *time.Timer
	var closeCh <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-closeCh:
			return
		case ev := <-ch:
			r.writeEvent(w, ev)
			flusher.Flush()
			if r.sseCloseDelay >= 0 && shouldCloseAfter(ev) {
				if r.sseCloseDelay == 0 {
					return
				}
				if closeTimer == nil {
					closeTimer = time.NewTimer(r.sseCloseDelay)
					closeCh = closeTimer.C
				}
			}
		}
	}
}

func shouldCloseAfter(ev Event) bool {
	if ev.Type != "status" {
		return false
	}
	switch v := ev.Data.(type) {
	case JobView:
		return isTerminalStatus(v.Status)
	case *JobView:
		if v == nil {
			return false
		}
		return isTerminalStatus(v.Status)
	case map[string]any:
		if status, ok := v["status"].(string); ok {
			return isTerminalStatus(status)
		}
	}
	return false
}

func isTerminalStatus(status string) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusCanceled:
		return true
	default:
		return false
	}
}

func (r *Registry) HandleCancelJob(w http.ResponseWriter, req *http.Request) {
	jobID := chi.URLParam(req, "job_id")
	var view JobView
	r.mu.Lock()
	job := r.jobs[jobID]
	if job == nil {
		r.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if job.Status == StatusQueued {
		r.removeFromQueue(jobID)
	}
	job.Status = StatusCanceled
	job.UpdatedAt = time.Now()
	view = r.viewLocked(job)
	r.mu.Unlock()
	r.clearClientTimer(jobID)
	r.publish(jobID, Event{Type: "status", Data: view})
	writeJSON(w, http.StatusOK, map[string]any{"status": "canceled"})
}

func (r *Registry) HandleClaimJob(w http.ResponseWriter, req *http.Request) {
	var body ClaimRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	wait := time.Duration(body.MaxWaitSeconds) * time.Second
	deadline := time.Now().Add(wait)
	workerID := strings.TrimSpace(body.WorkerID)
	workerGroup := strings.TrimSpace(body.WorkerGroup)
	r.recordWorkerSeen(workerID, workerGroup, "poll", body.Types, "")
	for {
		if job := r.claimNext(body.Types, workerID, workerGroup); job != nil {
			r.recordWorkerSeen(workerID, workerGroup, "poll", body.Types, job.ID)
			writeJSON(w, http.StatusOK, r.claimResponse(job))
			return
		}
		if wait <= 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		select {
		case <-r.notify:
		case <-time.After(remaining):
			w.WriteHeader(http.StatusNoContent)
			return
		case <-req.Context().Done():
			return
		}
	}
}

func (r *Registry) HandleClaimStream(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	types := parseListQuery(req.URL.Query()["types"])
	workerID := strings.TrimSpace(req.URL.Query().Get("worker_id"))
	workerGroup := strings.TrimSpace(req.URL.Query().Get("worker_group"))
	r.addWorkerStream(workerID, workerGroup, types)
	defer r.removeWorkerStream(workerID, workerGroup)
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	ctx := req.Context()
	for {
		if job := r.claimNext(types, workerID, workerGroup); job != nil {
			r.recordWorkerSeen(workerID, workerGroup, "stream", types, job.ID)
			r.writeEvent(w, Event{Type: "job", Data: r.claimResponse(job)})
			flusher.Flush()
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-r.notify:
			r.recordWorkerSeen(workerID, workerGroup, "stream", types, "")
		case <-keepAlive.C:
			r.recordWorkerSeen(workerID, workerGroup, "stream", types, "")
			_, _ = w.Write([]byte(": keep-alive\n\n"))
			flusher.Flush()
		}
	}
}

func (r *Registry) HandlePayloadRequest(w http.ResponseWriter, req *http.Request) {
	jobID := chi.URLParam(req, "job_id")
	var view JobView
	var body TransferRequest
	r.mu.Lock()
	job := r.jobs[jobID]
	if job == nil {
		r.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if !canRequestTransfer(job.Status) {
		r.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "invalid_state"})
		return
	}
	r.mu.Unlock()
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	key := "payload"
	if body.Key != nil {
		key = strings.TrimSpace(*body.Key)
	}
	channelID, expires := r.transfer.Create()
	readerURL := "/api/transfer/" + channelID
	writerURL := "/api/transfer/" + channelID

	info := &TransferInfo{
		ChannelID: channelID,
		Method:    http.MethodPost,
		URL:       writerURL,
		ExpiresAt: expires.UTC().Format(time.RFC3339),
		Key:       key,
	}
	r.mu.Lock()
	if job.Payloads == nil {
		job.Payloads = make(map[string]*TransferInfo)
	}
	job.Payloads[key] = info
	job.Status = StatusAwaitingPayload
	job.UpdatedAt = time.Now()
	view = r.viewLocked(job)
	r.mu.Unlock()

	r.publish(jobID, Event{Type: "payload", Data: info})
	r.publish(jobID, Event{Type: "status", Data: view})
	writeJSON(w, http.StatusOK, map[string]any{
		"key":        key,
		"channel_id": channelID,
		"reader_url": readerURL,
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

func (r *Registry) HandleResultRequest(w http.ResponseWriter, req *http.Request) {
	jobID := chi.URLParam(req, "job_id")
	var view JobView
	var body TransferRequest
	r.mu.Lock()
	job := r.jobs[jobID]
	if job == nil {
		r.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if !canRequestTransfer(job.Status) {
		r.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "invalid_state"})
		return
	}
	r.mu.Unlock()
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	key := "result"
	if body.Key != nil {
		key = strings.TrimSpace(*body.Key)
	}
	channelID, expires := r.transfer.Create()
	readerURL := "/api/transfer/" + channelID
	writerURL := "/api/transfer/" + channelID

	info := &TransferInfo{
		ChannelID: channelID,
		Method:    http.MethodGet,
		URL:       readerURL,
		ExpiresAt: expires.UTC().Format(time.RFC3339),
		Key:       key,
	}
	r.mu.Lock()
	if job.Results == nil {
		job.Results = make(map[string]*TransferInfo)
	}
	job.Results[key] = info
	job.Status = StatusAwaitingResult
	job.UpdatedAt = time.Now()
	view = r.viewLocked(job)
	r.mu.Unlock()

	r.publish(jobID, Event{Type: "result", Data: info})
	r.publish(jobID, Event{Type: "status", Data: view})
	writeJSON(w, http.StatusOK, map[string]any{
		"key":        key,
		"channel_id": channelID,
		"writer_url": writerURL,
		"expires_at": expires.UTC().Format(time.RFC3339),
	})
}

func (r *Registry) HandleStatusUpdate(w http.ResponseWriter, req *http.Request) {
	jobID := chi.URLParam(req, "job_id")
	var view JobView
	r.mu.Lock()
	job := r.jobs[jobID]
	if job == nil {
		r.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	if isTerminalStatus(job.Status) {
		r.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "invalid_state"})
		return
	}
	var body StatusUpdateRequest
	r.mu.Unlock()
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	state := strings.TrimSpace(body.State)
	if state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing_state"})
		return
	}
	r.mu.Lock()
	if isTerminalStatus(job.Status) {
		r.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"error": "invalid_state"})
		return
	}
	job.Status = state
	if body.Progress != nil {
		job.Progress = body.Progress
	}
	if body.Error != nil {
		job.Error = body.Error
	}
	job.UpdatedAt = time.Now()
	view = r.viewLocked(job)
	r.mu.Unlock()

	if isTerminalStatus(state) {
		r.clearClientTimer(jobID)
	}
	r.publish(jobID, Event{Type: "status", Data: view})
	writeJSON(w, http.StatusOK, map[string]any{"status": job.Status})
}

func (r *Registry) claimNext(types []string, workerID, workerGroup string) *Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, id := range r.queue {
		job := r.jobs[id]
		if job == nil {
			continue
		}
		if len(types) > 0 && !contains(types, job.Type) {
			continue
		}
		if !isCompatibleClaim(job, workerID, workerGroup) {
			continue
		}
		r.queue = append(r.queue[:i], r.queue[i+1:]...)
		job.Status = StatusClaimed
		job.ClaimedWorkerID = workerID
		job.ClaimedWorkerGroup = workerGroup
		job.UpdatedAt = time.Now()
		r.publishLocked(job.ID, Event{Type: "status", Data: r.viewLocked(job)})
		return job
	}
	return nil
}

func (r *Registry) removeFromQueue(jobID string) {
	for i, id := range r.queue {
		if id == jobID {
			r.queue = append(r.queue[:i], r.queue[i+1:]...)
			return
		}
	}
}

func (r *Registry) signal() {
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (r *Registry) subscribe(jobID string) chan Event {
	ch := make(chan Event, 8)
	r.mu.Lock()
	if r.subs[jobID] == nil {
		r.subs[jobID] = make(map[chan Event]struct{})
	}
	r.subs[jobID][ch] = struct{}{}
	r.mu.Unlock()
	return ch
}

func (r *Registry) unsubscribe(jobID string, ch chan Event) {
	r.mu.Lock()
	if subs := r.subs[jobID]; subs != nil {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(r.subs, jobID)
		}
	}
	r.mu.Unlock()
	close(ch)
}

func (r *Registry) publish(jobID string, ev Event) {
	r.mu.Lock()
	r.publishLocked(jobID, ev)
	r.mu.Unlock()
}

func (r *Registry) publishLocked(jobID string, ev Event) {
	subs := r.subs[jobID]
	for ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (r *Registry) onClientConnect(jobID string) {
	if r.clientTTL <= 0 {
		return
	}
	r.mu.Lock()
	if timer := r.clientTimers[jobID]; timer != nil {
		timer.Stop()
		delete(r.clientTimers, jobID)
	}
	r.mu.Unlock()
}

func (r *Registry) onClientDisconnect(jobID string) {
	r.startClientTimerIfIdle(jobID)
}

func (r *Registry) startClientTimerIfIdle(jobID string) {
	if r.clientTTL <= 0 {
		return
	}
	r.mu.Lock()
	subs := r.subs[jobID]
	if len(subs) > 0 {
		r.mu.Unlock()
		return
	}
	if r.clientTimers[jobID] != nil {
		r.mu.Unlock()
		return
	}
	ttl := r.clientTTL
	timer := time.AfterFunc(ttl, func() {
		r.expireJobIfIdle(jobID)
	})
	r.clientTimers[jobID] = timer
	r.mu.Unlock()
}

func (r *Registry) clearClientTimer(jobID string) {
	r.mu.Lock()
	if timer := r.clientTimers[jobID]; timer != nil {
		timer.Stop()
		delete(r.clientTimers, jobID)
	}
	r.mu.Unlock()
}

func (r *Registry) expireJobIfIdle(jobID string) {
	r.mu.Lock()
	subs := r.subs[jobID]
	if len(subs) > 0 {
		r.mu.Unlock()
		return
	}
	if r.clientTimers[jobID] == nil {
		r.mu.Unlock()
		return
	}
	delete(r.clientTimers, jobID)
	job := r.jobs[jobID]
	if job == nil {
		r.mu.Unlock()
		return
	}
	if isTerminalStatus(job.Status) {
		r.mu.Unlock()
		return
	}
	if job.Status == StatusQueued {
		r.removeFromQueue(jobID)
	}
	job.Status = StatusCanceled
	job.Error = &JobError{Code: "client_inactive", Message: "client inactive"}
	job.UpdatedAt = time.Now()
	view := r.viewLocked(job)
	r.mu.Unlock()

	logx.Log.Warn().Str("job_id", jobID).Msg("canceled job due to client inactivity")
	r.publish(jobID, Event{Type: "status", Data: view})
}

func (r *Registry) writeEvent(w http.ResponseWriter, ev Event) {
	data, err := json.Marshal(ev.Data)
	if err != nil {
		logx.Log.Warn().Err(err).Str("event", ev.Type).Msg("serialize job event")
		return
	}
	if ev.Type != "" {
		_, _ = w.Write([]byte("event: " + ev.Type + "\n"))
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n\n"))
}

func (r *Registry) snapshot(jobID string) (JobView, int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job := r.jobs[jobID]
	if job == nil {
		return JobView{}, 0, false
	}
	pos := 0
	if job.Status == StatusQueued {
		for i, id := range r.queue {
			if id == jobID {
				pos = i + 1
				break
			}
		}
	}
	return r.viewLocked(job), pos, true
}

func (r *Registry) viewLocked(job *Job) JobView {
	if job == nil {
		return JobView{}
	}
	return JobView{
		ID:                 job.ID,
		Type:               job.Type,
		Status:             job.Status,
		Metadata:           copyMap(job.Metadata),
		WorkerID:           job.WorkerID,
		WorkerGroup:        job.WorkerGroup,
		ClaimedWorkerID:    job.ClaimedWorkerID,
		ClaimedWorkerGroup: job.ClaimedWorkerGroup,
		Progress:           copyMap(job.Progress),
		Error:              copyError(job.Error),
		Payloads:           copyTransfers(job.Payloads),
		Results:            copyTransfers(job.Results),
		CreatedAt:          job.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:          job.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (r *Registry) claimResponse(job *Job) map[string]any {
	if job == nil {
		return map[string]any{}
	}
	return map[string]any{
		"job_id":               job.ID,
		"type":                 job.Type,
		"metadata":             copyMap(job.Metadata),
		"worker_id":            job.WorkerID,
		"worker_group":         job.WorkerGroup,
		"claimed_worker_id":    job.ClaimedWorkerID,
		"claimed_worker_group": job.ClaimedWorkerGroup,
	}
}

func (r *Registry) StateSnapshot() StateView {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	r.pruneWorkersLocked(now)

	state := StateView{
		Workers: make([]WorkerActivityView, 0, len(r.workers)),
		Jobs:    make([]JobView, 0, minInt(len(r.jobs), maxStateJobs)),
	}
	workerKeys := make([]string, 0, len(r.workers))
	for key := range r.workers {
		workerKeys = append(workerKeys, key)
	}
	sort.Strings(workerKeys)
	for _, key := range workerKeys {
		w := r.workers[key]
		if w == nil {
			continue
		}
		active := now.Sub(w.LastSeenAt) <= workerActiveWindow
		if active {
			state.Summary.ActiveWorkers++
		}
		state.Summary.RecentWorkers++
		if w.StreamCount > 0 {
			state.Summary.StreamingWorkers++
		}
		view := WorkerActivityView{
			Key:           w.Key,
			WorkerID:      w.WorkerID,
			WorkerGroup:   w.WorkerGroup,
			LastSeenAt:    w.LastSeenAt.UTC().Format(time.RFC3339),
			LastClaimMode: w.LastClaimMode,
			LastJobID:     w.LastJobID,
			Types:         append([]string(nil), w.Types...),
			StreamCount:   w.StreamCount,
			Active:        active,
		}
		if !w.LastClaimAt.IsZero() {
			view.LastClaimAt = w.LastClaimAt.UTC().Format(time.RFC3339)
		}
		state.Workers = append(state.Workers, view)
	}

	ids := make([]string, 0, len(r.jobs))
	for id := range r.jobs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return r.jobs[ids[i]].UpdatedAt.After(r.jobs[ids[j]].UpdatedAt)
	})
	for _, id := range ids {
		job := r.jobs[id]
		if job == nil {
			continue
		}
		state.Summary.TotalJobs++
		switch job.Status {
		case StatusQueued:
			state.Summary.QueuedJobs++
		case StatusClaimed:
			state.Summary.ClaimedJobs++
		case StatusRunning:
			state.Summary.RunningJobs++
		case StatusAwaitingPayload, StatusAwaitingResult:
			state.Summary.AwaitingTransfers++
		case StatusCompleted, StatusFailed, StatusCanceled:
			state.Summary.TerminalJobs++
		}
		if len(state.Jobs) < maxStateJobs {
			view := r.viewLocked(job)
			if job.Status == StatusQueued {
				for i, qid := range r.queue {
					if qid == id {
						view.QueuePosition = i + 1
						break
					}
				}
			}
			state.Jobs = append(state.Jobs, view)
		}
	}
	return state
}

func (r *Registry) StateHTML() string {
	return `
<div class="jobs-view">
  <style>
    .jobs-view { display: grid; gap: 1rem; }
    .jobs-summary { display: grid; gap: 0.75rem; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); }
    .jobs-card, .jobs-panel { border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--panel-strong); }
    .jobs-card { padding: 0.85rem 0.95rem; }
    .jobs-k { color: var(--muted); font-size: 0.76rem; letter-spacing: 0.06em; text-transform: uppercase; }
    .jobs-v { margin-top: 0.28rem; font-size: 1.45rem; font-weight: 700; line-height: 1; }
    .jobs-sub { margin-top: 0.25rem; color: var(--muted); font-size: 0.84rem; }
    .jobs-grid { display: grid; gap: 1rem; grid-template-columns: minmax(280px, 0.95fr) minmax(0, 1.4fr); }
    .jobs-panel { padding: 0.95rem 1rem; }
    .jobs-panel h4 { margin: 0 0 0.75rem; font-size: 0.96rem; }
    .jobs-list { display: grid; gap: 0.65rem; }
    .jobs-row { display: grid; gap: 0.15rem; padding: 0.7rem 0.75rem; border: 1px solid var(--border); border-radius: var(--radius-sm); background: rgba(255,255,255,0.04); }
    .jobs-row-head { display: flex; justify-content: space-between; gap: 0.75rem; align-items: center; }
    .jobs-id { font-family: var(--mono); font-size: 0.8rem; color: var(--muted); }
    .jobs-pill { display: inline-flex; align-items: center; padding: 0.16rem 0.46rem; border-radius: 999px; border: 1px solid var(--border); font-size: 0.76rem; color: var(--muted); }
    .jobs-pill.live { color: var(--ok); }
    .jobs-pill.warn { color: var(--warn); }
    .jobs-meta { color: var(--muted); font-size: 0.85rem; line-height: 1.35; }
    .jobs-table { width: 100%; border-collapse: collapse; font-size: 0.88rem; }
    .jobs-table th, .jobs-table td { text-align: left; padding: 0.56rem 0.45rem; border-bottom: 1px solid var(--border); vertical-align: top; }
    .jobs-table th { color: var(--muted); font-size: 0.74rem; text-transform: uppercase; letter-spacing: 0.06em; }
    .jobs-table td { color: var(--text); }
    .jobs-table code { font-family: var(--mono); font-size: 0.79rem; }
    .jobs-empty { color: var(--muted); font-size: 0.9rem; }
    @media (max-width: 900px) { .jobs-grid { grid-template-columns: 1fr; } }
  </style>
  <div class="jobs-summary"></div>
  <div class="jobs-grid">
    <section class="jobs-panel">
      <h4>Active Workers</h4>
      <div class="jobs-list jobs-workers"></div>
    </section>
    <section class="jobs-panel">
      <h4>Recent Jobs</h4>
      <div class="jobs-jobs"></div>
    </section>
  </div>
  <script>(function(){
    function esc(v){ return String(v == null ? '' : v).replace(/[&<>"]/g, function(c){ return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'})[c]; }); }
    function rel(ts){
      if (!ts) return 'never';
      var d = Math.max(0, Math.round((Date.now() - new Date(ts).getTime())/1000));
      if (d < 5) return 'just now';
      if (d < 60) return d + 's ago';
      if (d < 3600) return Math.floor(d/60) + 'm ago';
      return Math.floor(d/3600) + 'h ago';
    }
    function statusTone(s){
      s = String(s || '').toLowerCase();
      if (s === 'running' || s === 'claimed') return 'live';
      if (s === 'awaiting_payload' || s === 'awaiting_result' || s === 'queued') return 'warn';
      return '';
    }
    function render(state, container){
      state = state || {};
      var summary = state.summary || {};
      var workers = state.workers || [];
      var jobs = state.jobs || [];
      var summaryHost = container.querySelector('.jobs-summary');
      if (summaryHost) {
        summaryHost.innerHTML = [
          ['Active Workers', summary.active_workers || 0, 'Seen in the last minute'],
          ['Streaming', summary.streaming_workers || 0, 'Open worker SSE streams'],
          ['Queued', summary.queued_jobs || 0, 'Waiting for a compatible worker'],
          ['Running', summary.running_jobs || 0, 'Currently processing'],
          ['Awaiting Transfer', summary.awaiting_transfers || 0, 'Payload or result handoff'],
          ['Terminal', summary.terminal_jobs || 0, 'Completed, failed, or canceled']
        ].map(function(card){
          return '<article class="jobs-card"><div class="jobs-k">'+card[0]+'</div><div class="jobs-v">'+card[1]+'</div><div class="jobs-sub">'+card[2]+'</div></article>';
        }).join('');
      }
      var workersHost = container.querySelector('.jobs-workers');
      if (workersHost) {
        if (!workers.length) {
          workersHost.innerHTML = '<div class="jobs-empty">No recent worker claim activity.</div>';
        } else {
          workersHost.innerHTML = workers.map(function(w){
            var name = w.worker_id || '(group only)';
            var group = w.worker_group || 'none';
            var tone = w.active ? 'live' : 'warn';
            return '<div class="jobs-row">' +
              '<div class="jobs-row-head"><strong>'+esc(name)+'</strong><span class="jobs-pill '+tone+'">'+(w.active ? 'active' : 'recent')+'</span></div>' +
              '<div class="jobs-id">'+esc(group)+'</div>' +
              '<div class="jobs-meta">mode '+esc(w.last_claim_mode || 'unknown')+' | seen '+esc(rel(w.last_seen_at))+(w.stream_count ? ' | streams '+esc(w.stream_count) : '')+'</div>' +
              '<div class="jobs-meta">last job '+esc(w.last_job_id || 'none')+(w.types && w.types.length ? ' | types '+esc(w.types.join(', ')) : '')+'</div>' +
            '</div>';
          }).join('');
        }
      }
      var jobsHost = container.querySelector('.jobs-jobs');
      if (jobsHost) {
        if (!jobs.length) {
          jobsHost.innerHTML = '<div class="jobs-empty">No jobs recorded.</div>';
        } else {
          jobsHost.innerHTML = '<table class="jobs-table"><thead><tr><th>Status</th><th>Type</th><th>Affinity</th><th>Claimed By</th><th>Updated</th></tr></thead><tbody>' +
            jobs.map(function(j){
              var affinity = (j.worker_id || '-') + ' / ' + (j.worker_group || '-');
              var claimed = (j.claimed_worker_id || '-') + ' / ' + (j.claimed_worker_group || '-');
              var status = j.status || '';
              var extra = j.queue_position ? ' #' + j.queue_position : '';
              return '<tr>' +
                '<td><span class="jobs-pill '+statusTone(status)+'">'+esc(status + extra)+'</span></td>' +
                '<td><div>'+esc(j.type || '')+'</div><code>'+esc(j.id || '')+'</code></td>' +
                '<td>'+esc(affinity)+'</td>' +
                '<td>'+esc(claimed)+'</td>' +
                '<td>'+esc(rel(j.updated_at))+'</td>' +
              '</tr>';
            }).join('') +
            '</tbody></table>';
        }
      }
    }
    if (!window.NFRX) window.NFRX = { _renderers:{}, registerRenderer:function(id,fn){ this._renderers[id]=fn; } };
    var section = (document.currentScript && document.currentScript.closest('section')) || null;
    var id = (section && section.dataset && section.dataset.pluginId) || 'jobs';
    window.NFRX.registerRenderer(id, function(state, container){ render(state, container); });
  })();</script>
</div>`
}

func isCompatibleClaim(job *Job, workerID, workerGroup string) bool {
	if job == nil {
		return false
	}
	if job.WorkerID != "" && job.WorkerID != workerID {
		return false
	}
	if job.WorkerGroup != "" && job.WorkerGroup != workerGroup {
		return false
	}
	return true
}

func parseListQuery(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	res := make([]string, 0, len(values))
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				res = append(res, part)
			}
		}
	}
	if len(res) == 0 {
		return nil
	}
	return res
}

func (r *Registry) recordWorkerSeen(workerID, workerGroup, mode string, types []string, jobID string) {
	key := workerKey(workerID, workerGroup)
	if key == "" {
		return
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneWorkersLocked(now)
	w := r.ensureWorkerLocked(workerID, workerGroup)
	w.LastSeenAt = now
	if mode != "" {
		w.LastClaimMode = mode
	}
	if len(types) > 0 {
		w.Types = append([]string(nil), types...)
	}
	if jobID != "" {
		w.LastClaimAt = now
		w.LastJobID = jobID
	}
}

func (r *Registry) addWorkerStream(workerID, workerGroup string, types []string) {
	key := workerKey(workerID, workerGroup)
	if key == "" {
		return
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneWorkersLocked(now)
	w := r.ensureWorkerLocked(workerID, workerGroup)
	w.LastSeenAt = now
	w.LastClaimMode = "stream"
	if len(types) > 0 {
		w.Types = append([]string(nil), types...)
	}
	w.StreamCount++
}

func (r *Registry) removeWorkerStream(workerID, workerGroup string) {
	key := workerKey(workerID, workerGroup)
	if key == "" {
		return
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneWorkersLocked(now)
	if w := r.workers[key]; w != nil && w.StreamCount > 0 {
		w.StreamCount--
		w.LastSeenAt = now
	}
}

func (r *Registry) ensureWorkerLocked(workerID, workerGroup string) *WorkerActivity {
	key := workerKey(workerID, workerGroup)
	if key == "" {
		return nil
	}
	w := r.workers[key]
	if w == nil {
		w = &WorkerActivity{
			Key:         key,
			WorkerID:    workerID,
			WorkerGroup: workerGroup,
		}
		r.workers[key] = w
	}
	return w
}

func (r *Registry) pruneWorkersLocked(now time.Time) {
	for key, w := range r.workers {
		if w == nil {
			delete(r.workers, key)
			continue
		}
		if w.StreamCount > 0 {
			continue
		}
		if !w.LastSeenAt.IsZero() && now.Sub(w.LastSeenAt) > workerRetention {
			delete(r.workers, key)
		}
	}
}

func workerKey(workerID, workerGroup string) string {
	workerID = strings.TrimSpace(workerID)
	workerGroup = strings.TrimSpace(workerGroup)
	if workerID == "" && workerGroup == "" {
		return ""
	}
	return workerID + "::" + workerGroup
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func contains(list []string, v string) bool {
	for _, it := range list {
		if it == v {
			return true
		}
	}
	return false
}

func canRequestTransfer(state string) bool {
	switch state {
	case StatusClaimed, StatusRunning, StatusAwaitingPayload, StatusAwaitingResult:
		return true
	default:
		return false
	}
}

func copyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func copyError(err *JobError) *JobError {
	if err == nil {
		return nil
	}
	cp := *err
	return &cp
}

func copyTransfer(info *TransferInfo) *TransferInfo {
	if info == nil {
		return nil
	}
	cp := *info
	return &cp
}

func copyTransfers(src map[string]*TransferInfo) map[string]*TransferInfo {
	if src == nil {
		return nil
	}
	dst := make(map[string]*TransferInfo, len(src))
	for k, v := range src {
		dst[k] = copyTransfer(v)
	}
	return dst
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
