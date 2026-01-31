package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

const defaultTransferTTL = 60 * time.Second

var (
	errTransferNotFound  = errors.New("transfer not found")
	errTransferExpired   = errors.New("transfer expired")
	errTransferRoleTaken = errors.New("transfer role already attached")
	errTransferClosed    = errors.New("transfer closed")
)

type transferRole int

const (
	transferReader transferRole = iota
	transferWriter
)

type TransferRegistry struct {
	mu       sync.Mutex
	channels map[string]*transferChannel
	ttl      time.Duration
}

type transferChannel struct {
	id        string
	expiresAt time.Time
	reader    *transferEndpoint
	writer    *transferEndpoint
	ready     chan struct{}
	done      chan struct{}
	timer     *time.Timer
	mu        sync.Mutex
	closed    bool
	err       error
}

type transferEndpoint struct {
	w http.ResponseWriter
	r *http.Request
}

func NewTransferRegistry(ttl time.Duration) *TransferRegistry {
	if ttl <= 0 {
		ttl = defaultTransferTTL
	}
	return &TransferRegistry{channels: make(map[string]*transferChannel), ttl: ttl}
}

func (tr *TransferRegistry) Create() (string, time.Time) {
	id := uuid.NewString()
	expires := time.Now().Add(tr.ttl)
	ch := &transferChannel{
		id:        id,
		expiresAt: expires,
		ready:     make(chan struct{}),
		done:      make(chan struct{}),
	}
	ch.timer = time.AfterFunc(tr.ttl, func() {
		tr.expire(id)
	})
	tr.mu.Lock()
	tr.channels[id] = ch
	tr.mu.Unlock()
	return id, expires
}

func (tr *TransferRegistry) expire(id string) {
	tr.mu.Lock()
	ch := tr.channels[id]
	if ch == nil {
		tr.mu.Unlock()
		return
	}
	delete(tr.channels, id)
	tr.mu.Unlock()
	ch.close(errTransferExpired)
}

func (tr *TransferRegistry) Attach(id string, role transferRole, ep *transferEndpoint) (*transferChannel, error) {
	tr.mu.Lock()
	ch := tr.channels[id]
	tr.mu.Unlock()
	if ch == nil {
		return nil, errTransferNotFound
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.closed {
		if errors.Is(ch.err, errTransferExpired) {
			return nil, errTransferExpired
		}
		return nil, errTransferClosed
	}
	switch role {
	case transferReader:
		if ch.reader != nil {
			return nil, errTransferRoleTaken
		}
		ch.reader = ep
	case transferWriter:
		if ch.writer != nil {
			return nil, errTransferRoleTaken
		}
		ch.writer = ep
	default:
		return nil, errTransferClosed
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

func (tr *TransferRegistry) Close(id string, err error) {
	tr.mu.Lock()
	ch := tr.channels[id]
	if ch != nil {
		delete(tr.channels, id)
	}
	tr.mu.Unlock()
	if ch != nil {
		ch.close(err)
	}
}

func (ch *transferChannel) close(err error) {
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

func (ch *transferChannel) error() error {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.err
}

func (tr *TransferRegistry) HandleCreate(w http.ResponseWriter, r *http.Request) {
	id, expires := tr.Create()
	resp := map[string]any{
		"channel_id": id,
		"expires_at": expires.UTC().Format(time.RFC3339),
	}
	writeJSON(w, http.StatusOK, resp)
}

func (tr *TransferRegistry) HandleReader(w http.ResponseWriter, r *http.Request, id string) {
	ep := &transferEndpoint{w: w, r: r}
	ch, err := tr.Attach(id, transferReader, ep)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")

	ctx := r.Context()
	select {
	case <-ch.ready:
		// Wait for transfer to complete; writer will stream into this response.
	case <-ch.done:
		writeTransferError(w, ch.error())
		return
	case <-ctx.Done():
		tr.Close(id, ctx.Err())
		return
	}
	<-ch.done
}

func (tr *TransferRegistry) HandleWriter(w http.ResponseWriter, r *http.Request, id string) {
	ep := &transferEndpoint{w: w, r: r}
	ch, err := tr.Attach(id, transferWriter, ep)
	if err != nil {
		writeTransferError(w, err)
		return
	}
	ctx := r.Context()
	select {
	case <-ch.ready:
	case <-ch.done:
		writeTransferError(w, ch.error())
		return
	case <-ctx.Done():
		tr.Close(id, ctx.Err())
		return
	}
	copyErr := pipeTransfer(ch)
	tr.Close(id, copyErr)
	if copyErr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "transfer_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func pipeTransfer(ch *transferChannel) error {
	ch.mu.Lock()
	reader := ch.reader
	writer := ch.writer
	ch.mu.Unlock()
	if reader == nil || writer == nil {
		return errTransferClosed
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

func writeTransferError(w http.ResponseWriter, err error) {
	if err == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_transfer"})
		return
	}
	switch {
	case errors.Is(err, errTransferNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "transfer_not_found"})
	case errors.Is(err, errTransferExpired):
		writeJSON(w, http.StatusGone, map[string]any{"error": "transfer_expired"})
	case errors.Is(err, errTransferRoleTaken):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "transfer_in_use"})
	case errors.Is(err, errTransferClosed):
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
