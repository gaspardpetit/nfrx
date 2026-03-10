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
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Model     string `json:"model"`
		Position  int    `json:"position"`
	}
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if msg.Type != "nfrx.queue" || msg.RequestID != "req1" || msg.Model != "m" {
		t.Fatalf("unexpected queue payload %+v", msg)
	}
	if got := msg.Position; got != 2 {
		t.Fatalf("expected position=2, got %d", got)
	}
}

func TestCompletionQueueFirstDispatchableSkipsBlockedEntries(t *testing.T) {
	q := NewCompletionQueue(nil, 4)
	if _, ok := q.Enter("req-a", "model-a"); !ok {
		t.Fatalf("expected req-a to enter queue")
	}
	if _, ok := q.Enter("req-b", "model-b"); !ok {
		t.Fatalf("expected req-b to enter queue")
	}
	canDispatch := func(model string) bool {
		return model == "model-b"
	}
	if q.IsFirstDispatchable("req-a", canDispatch) {
		t.Fatalf("req-a should not be dispatchable")
	}
	if !q.IsFirstDispatchable("req-b", canDispatch) {
		t.Fatalf("req-b should be first dispatchable entry")
	}
}
