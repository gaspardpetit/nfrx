package ctrl

import "testing"

func TestLeastBusyScheduler(t *testing.T) {
	reg := NewRegistry()
	w1 := &Worker{ID: "w1", Models: map[string]bool{"m": true}, InFlight: 1}
	w2 := &Worker{ID: "w2", Models: map[string]bool{"m": true}, InFlight: 0}
	reg.Add(w1)
	reg.Add(w2)
	sched := &LeastBusyScheduler{Reg: reg}
	w, err := sched.PickWorker("m")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if w.ID != "w2" {
		t.Fatalf("expected w2, got %s", w.ID)
	}
}
