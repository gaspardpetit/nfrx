package ctrl

import "testing"

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	w := &Worker{ID: "w1", Models: map[string]bool{"m": true}}
	reg.Add(w)
	if len(reg.WorkersForModel("m")) != 1 {
		t.Fatalf("expected worker for model")
	}
	reg.UpdateHeartbeat("w1")
	if reg.workers["w1"].LastHeartbeat.IsZero() {
		t.Fatalf("heartbeat not updated")
	}
	reg.Remove("w1")
	if len(reg.WorkersForModel("m")) != 0 {
		t.Fatalf("expected no workers after remove")
	}
}
