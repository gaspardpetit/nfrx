package jobs

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
}

type Job struct {
	ID        string
	Type      string
	Status    string
	Metadata  map[string]any
	Progress  map[string]any
	Error     *JobError
	Payloads  map[string]*TransferInfo
	Results   map[string]*TransferInfo
	CreatedAt time.Time
	UpdatedAt time.Time
}

type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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
	Type     string         `json:"type"`
	Priority int            `json:"priority,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ClaimRequest struct {
	Types          []string `json:"types,omitempty"`
	MaxWaitSeconds int      `json:"max_wait_seconds,omitempty"`
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
	ID            string                   `json:"id"`
	Type          string                   `json:"type"`
	Status        string                   `json:"status"`
	Metadata      map[string]any           `json:"metadata,omitempty"`
	Progress      map[string]any           `json:"progress,omitempty"`
	Error         *JobError                `json:"error,omitempty"`
	Payloads      map[string]*TransferInfo `json:"payloads,omitempty"`
	Results       map[string]*TransferInfo `json:"results,omitempty"`
	QueuePosition int                      `json:"queue_position,omitempty"`
	CreatedAt     string                   `json:"created_at"`
	UpdatedAt     string                   `json:"updated_at"`
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
		ID:        uuid.NewString(),
		Type:      body.Type,
		Status:    StatusQueued,
		Metadata:  body.Metadata,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
	for {
		if job := r.claimNext(body.Types); job != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"job_id":   job.ID,
				"type":     job.Type,
				"metadata": job.Metadata,
			})
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

func (r *Registry) claimNext(types []string) *Job {
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
		r.queue = append(r.queue[:i], r.queue[i+1:]...)
		job.Status = StatusClaimed
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
		ID:        job.ID,
		Type:      job.Type,
		Status:    job.Status,
		Metadata:  copyMap(job.Metadata),
		Progress:  copyMap(job.Progress),
		Error:     copyError(job.Error),
		Payloads:  copyTransfers(job.Payloads),
		Results:   copyTransfers(job.Results),
		CreatedAt: job.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: job.UpdatedAt.UTC().Format(time.RFC3339),
	}
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
