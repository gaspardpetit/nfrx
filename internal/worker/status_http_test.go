package worker

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusHTTP(t *testing.T) {
	resetState()
	SetBuildInfo("v1", "sha1", "2024-01-01")
	SetWorkerInfo("id1", "worker", 2, []string{"m1"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfgFile := filepath.Join(t.TempDir(), "worker.yaml")
	addr, err := StartStatusServer(ctx, "127.0.0.1:0", cfgFile, time.Second, cancel)
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

func TestControlEndpoints(t *testing.T) {
	resetState()
	SetConnectedToServer(true)
	SetState("connected_idle")
	ctx, cancel := context.WithCancel(context.Background())
	cfgFile := filepath.Join(t.TempDir(), "worker.yaml")
	addr, err := StartStatusServer(ctx, "127.0.0.1:0", cfgFile, 100*time.Millisecond, cancel)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	tokenBytes, err := os.ReadFile(filepath.Join(filepath.Dir(cfgFile), "worker.token"))
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/control/drain", nil)
	req.Header.Set("X-Auth-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("drain request failed: %v %v", err, resp.Status)
	}
	_ = resp.Body.Close()
	if !IsDraining() {
		t.Fatalf("expected draining")
	}

	req, _ = http.NewRequest(http.MethodPost, "http://"+addr+"/control/undrain", nil)
	req.Header.Set("X-Auth-Token", token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("undrain request failed: %v %v", err, resp.Status)
	}
	_ = resp.Body.Close()
	if IsDraining() {
		t.Fatalf("expected not draining")
	}

	req, _ = http.NewRequest(http.MethodPost, "http://"+addr+"/control/shutdown", nil)
	req.Header.Set("X-Auth-Token", token)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("shutdown request failed: %v %v", err, resp.Status)
	}
	_ = resp.Body.Close()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("context not canceled")
	}
}
