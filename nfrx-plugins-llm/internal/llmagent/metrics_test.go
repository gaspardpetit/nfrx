package llmagent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMetricsServer(t *testing.T) {
	resetState()
	SetWorkerInfo("id1", "worker", 2, 0, nil)
	SetConnectedToServer(true)
	SetConnectedToBackend(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, err := StartMetricsServer(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start metrics server: %v", err)
	}
	JobStarted()
	JobCompleted(true, 10*time.Millisecond)
	resp, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	data := string(body)
	if !strings.Contains(data, "nfrx_worker_connected_to_server 1") {
		t.Fatalf("missing connected_to_server gauge: %s", data)
	}
	if !strings.Contains(data, "nfrx_worker_connected_to_backend 1") {
		t.Fatalf("missing connected_to_backend gauge: %s", data)
	}
	if !strings.Contains(data, "nfrx_worker_jobs_started_total") {
		t.Fatalf("missing jobs_started_total counter: %s", data)
	}
	if !strings.Contains(data, "nfrx_worker_job_duration_seconds") {
		t.Fatalf("missing job duration histogram: %s", data)
	}
}
