package serverstate

import "testing"

func TestMemoryStore(t *testing.T) {
	ms := NewMemoryStore()

	// Swap in the test store and restore the previous one after the test.
	prev := active
	UseStore(ms)
	defer UseStore(prev)

	if got := GetState(); got != "not_ready" {
		t.Fatalf("initial state = %q; want %q", got, "not_ready")
	}
	if IsDraining() {
		t.Fatalf("initial draining = true; want false")
	}

	SetState("ready")
	if got := GetState(); got != "ready" {
		t.Fatalf("state after SetState = %q; want %q", got, "ready")
	}

	StartDrain()
	if got := GetState(); got != "draining" {
		t.Fatalf("state after StartDrain = %q; want %q", got, "draining")
	}
	if !IsDraining() {
		t.Fatalf("IsDraining = false; want true")
	}
}
