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

func TestLeastBusySchedulerExactMatchWins(t *testing.T) {
	reg := NewRegistry()
	exact := &Worker{ID: "exact", Models: map[string]bool{"llama2:7b-q4_0": true}, InFlight: 10}
	alias := &Worker{ID: "alias", Models: map[string]bool{"llama2:7b-fp16": true}, InFlight: 0}
	reg.Add(exact)
	reg.Add(alias)
	sched := &LeastBusyScheduler{Reg: reg}
	w, err := sched.PickWorker("llama2:7b-q4_0")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if w.ID != "exact" {
		t.Fatalf("expected exact, got %s", w.ID)
	}
}

func TestLeastBusySchedulerAliasFallback(t *testing.T) {
	reg := NewRegistry()
	alias := &Worker{ID: "alias", Models: map[string]bool{"llama2:7b-fp16": true}}
	reg.Add(alias)
	sched := &LeastBusyScheduler{Reg: reg}
	w, err := sched.PickWorker("llama2:7b-q4_0")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if w.ID != "alias" {
		t.Fatalf("expected alias, got %s", w.ID)
	}
}

func TestLeastBusySchedulerNoAliasCandidates(t *testing.T) {
	reg := NewRegistry()
	reg.Add(&Worker{ID: "w1", Models: map[string]bool{"mistral:7b-q4_0": true}})
	sched := &LeastBusyScheduler{Reg: reg}
	if _, err := sched.PickWorker("llama2:7b-q4_0"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLeastBusySchedulerNoDashAlias(t *testing.T) {
	reg := NewRegistry()
	w := &Worker{ID: "w1", Models: map[string]bool{"mistral:7b-fp16": true}}
	reg.Add(w)
	sched := &LeastBusyScheduler{Reg: reg}
	wkr, err := sched.PickWorker("mistral:7b")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if wkr.ID != "w1" {
		t.Fatalf("expected w1, got %s", wkr.ID)
	}
}

func TestLeastBusySchedulerAliasLeastBusy(t *testing.T) {
	reg := NewRegistry()
	w1 := &Worker{ID: "w1", Models: map[string]bool{"llama2:7b-q4_0": true}, InFlight: 2}
	w2 := &Worker{ID: "w2", Models: map[string]bool{"llama2:7b-fp16": true}, InFlight: 1}
	reg.Add(w1)
	reg.Add(w2)
	sched := &LeastBusyScheduler{Reg: reg}
	w, err := sched.PickWorker("llama2:7b-q5_0")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if w.ID != "w2" {
		t.Fatalf("expected w2, got %s", w.ID)
	}
}
