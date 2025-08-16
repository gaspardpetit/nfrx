package ctrl

import (
	"sync"
	"testing"
	"time"
)

func TestSnapshotEmpty(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	snap := reg.Snapshot()
	if snap.Server.Version != "v" {
		t.Fatalf("version mismatch")
	}
	if snap.Server.JobsInflight != 0 || snap.Server.JobsCompletedTotal != 0 {
		t.Fatalf("expected zero jobs")
	}
	// uptime may be zero immediately after creation
}

func TestWorkerLifecycle(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	reg.UpsertWorker("w1", "w1", "1.0", "a", "today", 1, []string{"llama3:8b"})
	reg.SetWorkerStatus("w1", StatusConnected)
	reg.RecordHeartbeat("w1")
	reg.RecordJobStart("w1")
	reg.RecordJobEnd("w1", "llama3:8b", 100*time.Millisecond, 10, 20, true, "")

	snap := reg.Snapshot()
	if len(snap.Workers) != 1 {
		t.Fatalf("expected one worker")
	}
	w := snap.Workers[0]
	if w.ProcessedTotal != 1 || w.Inflight != 0 {
		t.Fatalf("bad worker counts %+v", w)
	}
	if w.PerModel["llama3:8b"].SuccessTotal != 1 {
		t.Fatalf("expected per-model success")
	}
	if snap.Server.JobsCompletedTotal != 1 {
		t.Fatalf("expected job completed")
	}
}

func TestErrorPaths(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	reg.UpsertWorker("w1", "w1", "1.0", "a", "today", 1, nil)
	reg.RecordJobStart("w1")
	reg.RecordJobEnd("w1", "m", 0, 0, 0, false, "boom")

	snap := reg.Snapshot()
	w := snap.Workers[0]
	if w.FailuresTotal != 1 || w.LastError != "boom" {
		t.Fatalf("expected failure recorded")
	}
	if snap.Server.JobsFailedTotal != 1 {
		t.Fatalf("expected global failure")
	}
}

func TestWorkersSummaryAndModels(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	reg.UpsertWorker("a", "a", "1", "", "", 1, []string{"m1", "m2"})
	reg.SetWorkerStatus("a", StatusConnected)
	reg.UpsertWorker("b", "b", "1", "", "", 1, []string{"m2"})
	reg.SetWorkerStatus("b", StatusIdle)

	snap := reg.Snapshot()
	if snap.WorkersSummary.Connected != 1 || snap.WorkersSummary.Idle != 1 {
		t.Fatalf("bad summary %+v", snap.WorkersSummary)
	}
	if len(snap.Models) != 2 {
		t.Fatalf("expected two models")
	}
}

func TestRemoveWorker(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	reg.UpsertWorker("w1", "w1", "1", "", "", 1, nil)
	reg.RemoveWorker("w1")
	snap := reg.Snapshot()
	if len(snap.Workers) != 0 {
		t.Fatalf("expected no workers, got %d", len(snap.Workers))
	}
}

func TestWorkersSortedByAge(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	reg.UpsertWorker("old", "old", "1", "", "", 1, nil)
	time.Sleep(10 * time.Millisecond)
	reg.UpsertWorker("new", "new", "1", "", "", 1, nil)
	snap := reg.Snapshot()
	if len(snap.Workers) != 2 || snap.Workers[0].ID != "old" {
		t.Fatalf("expected deterministic sort by age, got %+v", snap.Workers)
	}
}

func TestRegistryRace(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	reg.UpsertWorker("w", "w", "1", "", "", 1, []string{"m"})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				reg.RecordHeartbeat("w")
				reg.RecordJobStart("w")
				reg.RecordJobEnd("w", "m", time.Millisecond, 0, 0, true, "")
			}
		}()
	}
	wg.Wait()
}
