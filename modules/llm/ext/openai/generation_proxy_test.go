package openai

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

type testFlushRecorder struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (f *testFlushRecorder) Flush() { f.flushed = true }

func TestQueueStatusWriterUsesDedicatedEvent(t *testing.T) {
	rec := &testFlushRecorder{ResponseRecorder: httptest.NewRecorder()}
	if ok := queueStatusWriter(rec, rec, "req1", "m", 2); !ok {
		t.Fatalf("expected queued status to be written")
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "event: nfrx.queue\n") {
		t.Fatalf("queue payload missing dedicated event header: %q", body)
	}
	payload := strings.TrimPrefix(body, "event: nfrx.queue\n")
	payload = strings.TrimPrefix(payload, "data: ")
	payload = strings.TrimSuffix(payload, "\n\n")
	var msg struct {
		RequestID string `json:"request_id"`
		Model     string `json:"model"`
		Position  int    `json:"position"`
	}
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if msg.RequestID != "req1" || msg.Model != "m" {
		t.Fatalf("unexpected queue payload %+v", msg)
	}
	if got := msg.Position; got != 2 {
		t.Fatalf("expected position=2, got %d", got)
	}
}
