package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPromMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	Register(reg)
	SetServerBuildInfo("1.0.0", "abc", "2024-01-01")
	RecordModelRequest("llama3:8b", true)
	RecordModelTokens("llama3:8b", "in", 10)
	ObserveRequestDuration("w1", "llama3:8b", 100*time.Millisecond)
	RecordWorkerTokens("w1", "in", 10)
	RecordWorkerProcessingTime("w1", 100*time.Millisecond)

	if v := testutil.ToFloat64(modelRequests.WithLabelValues("llama3:8b", "success")); v != 1 {
		t.Fatalf("model requests: %v", v)
	}
	if v := testutil.ToFloat64(modelTokens.WithLabelValues("in", "llama3:8b")); v != 10 {
		t.Fatalf("model tokens: %v", v)
	}
	if v := testutil.ToFloat64(workerTokens.WithLabelValues("w1", "in")); v != 10 {
		t.Fatalf("worker tokens: %v", v)
	}
	if v := testutil.ToFloat64(workerProcessing.WithLabelValues("w1")); v != 0.1 {
		t.Fatalf("worker processing: %v", v)
	}
	if v := testutil.ToFloat64(buildInfo.WithLabelValues("2024-01-01", "abc", "1.0.0")); v != 1 {
		t.Fatalf("build info: %v", v)
	}
}
