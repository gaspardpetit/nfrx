package transfer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

const DefaultTTL = 60 * time.Second

var (
	ErrNotFound  = errors.New("transfer not found")
	ErrExpired   = errors.New("transfer expired")
	ErrRoleTaken = errors.New("transfer role already attached")
	ErrClosed    = errors.New("transfer closed")
)

type Role int

const (
	Reader Role = iota
	Writer
)

type Registry struct {
	mu       sync.Mutex
	channels map[string]*channel
	ttl      time.Duration
}

type channel struct {
	id        string
	expiresAt time.Time
	reader    *endpoint
	writer    *endpoint
	ready     chan struct{}
	done      chan struct{}
	timer     *time.Timer
	mu        sync.Mutex
	closed    bool
	err       error
}

type endpoint struct {
	w http.ResponseWriter
	r *http.Request
}

func NewRegistry(ttl time.Duration) *Registry {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Registry{channels: make(map[string]*channel), ttl: ttl}
}

func (r *Registry) Create() (string, time.Time) {
	id := uuid.NewString()
	expires := time.Now().Add(r.ttl)
	ch := &channel{
		id:        id,
		expiresAt: expires,
		ready:     make(chan struct{}),
		done:      make(chan struct{}),
	}
	ch.timer = time.AfterFunc(r.ttl, func() {
		r.expire(id)
	})
	r.mu.Lock()
	r.channels[id] = ch
	r.mu.Unlock()
	return id, expires
}

func (r *Registry) expire(id string) {
	r.mu.Lock()
	ch := r.channels[id]
	if ch == nil {
		r.mu.Unlock()
		return
	}
	delete(r.channels, id)
	r.mu.Unlock()
	ch.close(ErrExpired)
}

func (r *Registry) Attach(id string, role Role, ep *endpoint) (*channel, error) {
	r.mu.Lock()
	ch := r.channels[id]
	r.mu.Unlock()
	if ch == nil {
		return nil, ErrNotFound
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.closed {
		if errors.Is(ch.err, ErrExpired) {
			return nil, ErrExpired
		}
		return nil, ErrClosed
	}
	switch role {
	case Reader:
		if ch.reader != nil {
			return nil, ErrRoleTaken
		}
		ch.reader = ep
	case Writer:
		if ch.writer != nil {
			return nil, ErrRoleTaken
		}
		ch.writer = ep
	default:
		return nil, ErrClosed
	}
	if ch.reader != nil && ch.writer != nil {
		if ch.timer != nil {
			ch.timer.Stop()
			ch.timer = nil
		}
		select {
		case <-ch.ready:
		default:
			close(ch.ready)
		}
	}
	return ch, nil
}

func (r *Registry) Close(id string, err error) {
	r.mu.Lock()
	ch := r.channels[id]
	if ch != nil {
		delete(r.channels, id)
	}
	r.mu.Unlock()
	if ch != nil {
		ch.close(err)
	}
}

func (ch *channel) close(err error) {
	ch.mu.Lock()
	if ch.closed {
		ch.mu.Unlock()
		return
	}
	ch.closed = true
	ch.err = err
	ch.mu.Unlock()
	close(ch.done)
}

func (ch *channel) error() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.err
}

func (r *Registry) HandleCreate(w http.ResponseWriter, _ *http.Request) {
	id, expires := r.Create()
	resp := map[string]any{
		"channel_id": id,
		"expires_at": expires.UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (r *Registry) HandleReader(w http.ResponseWriter, req *http.Request, id string) {
	ep := &endpoint{w: w, r: req}
	ch, err := r.Attach(id, Reader, ep)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")

	ctx := req.Context()
	select {
	case <-ch.ready:
	case <-ch.done:
		writeError(w, ch.error())
		return
	case <-ctx.Done():
		r.Close(id, ctx.Err())
		return
	}
	<-ch.done
}

func (r *Registry) HandleWriter(w http.ResponseWriter, req *http.Request, id string) {
	ep := &endpoint{w: w, r: req}
	ch, err := r.Attach(id, Writer, ep)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx := req.Context()
	select {
	case <-ch.ready:
	case <-ch.done:
		writeError(w, ch.error())
		return
	case <-ctx.Done():
		r.Close(id, ctx.Err())
		return
	}
	copyErr := pipe(ch)
	r.Close(id, copyErr)
	if copyErr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "transfer_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func pipe(ch *channel) error {
	ch.mu.Lock()
	reader := ch.reader
	writer := ch.writer
	ch.mu.Unlock()
	if reader == nil || writer == nil {
		return ErrClosed
	}
	flusher, _ := reader.w.(http.Flusher)
	dst := io.Writer(reader.w)
	if flusher != nil {
		dst = &flushWriter{w: reader.w, f: flusher}
	}
	buf := make([]byte, 32*1024)
	_, err := io.CopyBuffer(dst, writer.r.Body, buf)
	return err
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}

func writeError(w http.ResponseWriter, err error) {
	if err == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_transfer"})
		return
	}
	switch {
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "transfer_not_found"})
	case errors.Is(err, ErrExpired):
		writeJSON(w, http.StatusGone, map[string]any{"error": "transfer_expired"})
	case errors.Is(err, ErrRoleTaken):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "transfer_in_use"})
	case errors.Is(err, ErrClosed):
		writeJSON(w, http.StatusGone, map[string]any{"error": "transfer_closed"})
	default:
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "transfer_failed"})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
