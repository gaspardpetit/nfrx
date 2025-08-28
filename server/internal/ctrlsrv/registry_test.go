package ctrlsrv

import (
	"testing"
	"time"
)

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	w := &Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
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

func TestRegistryPruneExpired(t *testing.T) {
	reg := NewRegistry()
	w := &Worker{ID: "w1", Models: map[string]bool{"m": true}, MaxConcurrency: 1, EmbeddingBatchSize: 0, LastHeartbeat: time.Now().Add(-HeartbeatExpiry - time.Second), Send: make(chan interface{}), Jobs: make(map[string]chan interface{})}
	jobCh := make(chan interface{})
	w.Jobs["j1"] = jobCh
	reg.Add(w)
	reg.PruneExpired(HeartbeatExpiry)
	if len(reg.WorkersForModel("m")) != 0 {
		t.Fatalf("expected worker pruned")
	}
	if _, ok := <-w.Send; ok {
		t.Fatalf("expected send channel closed")
	}
	if _, ok := <-jobCh; ok {
		t.Fatalf("expected job channel closed")
	}
}

func TestRegistryWorkersForAlias(t *testing.T) {
	reg := NewRegistry()
	w := &Worker{ID: "w1", Models: map[string]bool{"llama2:7b-fp16": true}, MaxConcurrency: 1, EmbeddingBatchSize: 0}
	reg.Add(w)
	ws := reg.WorkersForAlias("llama2:7b-q4_0")
	if len(ws) != 1 || ws[0].ID != "w1" {
		t.Fatalf("expected alias worker")
	}
}
