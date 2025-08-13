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
	if snap.Server.UptimeSeconds < 0 {
		t.Fatalf("invalid uptime")
	}
}

func TestWorkerLifecycle(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	reg.UpsertWorker("w1", "1.0", "a", "today", []string{"llama3:8b"})
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
	reg.UpsertWorker("w1", "1.0", "a", "today", nil)
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
	reg.UpsertWorker("a", "1", "", "", []string{"m1", "m2"})
	reg.SetWorkerStatus("a", StatusConnected)
	reg.UpsertWorker("b", "1", "", "", []string{"m2"})
	reg.SetWorkerStatus("b", StatusIdle)

	snap := reg.Snapshot()
	if snap.WorkersSummary.Connected != 1 || snap.WorkersSummary.Idle != 1 {
		t.Fatalf("bad summary %+v", snap.WorkersSummary)
	}
	if len(snap.Models) != 2 {
		t.Fatalf("expected two models")
	}
}

func TestRegistryRace(t *testing.T) {
	reg := NewMetricsRegistry("v", "sha", "date")
	reg.UpsertWorker("w", "1", "", "", []string{"m"})
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
