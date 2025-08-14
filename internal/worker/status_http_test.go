package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestStatusHTTP(t *testing.T) {
	resetState()
	SetBuildInfo("v1", "sha1", "2024-01-01")
	SetWorkerInfo("id1", "worker", 2, []string{"m1"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr, err := StartStatusServer(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	SetConnectedToServer(true)
	SetState("connected_idle")
	resp, err := http.Get("http://" + addr + "/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var st State
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.State != "connected_idle" || !st.ConnectedToServer {
		t.Fatalf("unexpected state: %+v", st)
	}
	respV, err := http.Get("http://" + addr + "/version")
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	defer func() { _ = respV.Body.Close() }()
	var vi VersionInfo
	if err := json.NewDecoder(respV.Body).Decode(&vi); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	if vi.Version != "v1" || vi.BuildSHA != "sha1" {
		t.Fatalf("unexpected version info: %+v", vi)
	}
}
