package workerproxy

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

func TestStatusAndControl(t *testing.T) {
	// Initialize some state
	SetBuildInfo("v1", "sha1", "2025-01-01")
	SetWorkerInfo("id1", "worker", 2)
	SetConnectedToServer(true)
	SetState("connected_idle")

	ctx, cancel := context.WithCancel(context.Background())
	cfgFile := filepath.Join(t.TempDir(), "agent.yaml")
	addr, err := StartStatusServer(ctx, "127.0.0.1:0", "wp", cfgFile, 100*time.Millisecond, cancel)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}

	// Status
	resp, err := http.Get("http://" + addr + "/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	defer resp.Body.Close()
	var st State
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.State != "connected_idle" || !st.ConnectedToServer {
		t.Fatalf("unexpected state: %+v", st)
	}

	// Version
	respV, err := http.Get("http://" + addr + "/version")
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	defer respV.Body.Close()
	var vi VersionInfo
	if err := json.NewDecoder(respV.Body).Decode(&vi); err != nil {
		t.Fatalf("decode version: %v", err)
	}
	if vi.Version != "v1" || vi.BuildSHA != "sha1" {
		t.Fatalf("unexpected version: %+v", vi)
	}

	// Auth token
	tok, err := os.ReadFile(filepath.Join(filepath.Dir(cfgFile), "wp.token"))
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	token := strings.TrimSpace(string(tok))

	// Drain
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/control/drain", nil)
	req.Header.Set("X-Auth-Token", token)
	r, err := http.DefaultClient.Do(req)
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("drain: %v %v", err, r.Status)
	}
	_ = r.Body.Close()
	if !IsDraining() {
		t.Fatalf("expected draining")
	}

	// Undrain
	req, _ = http.NewRequest(http.MethodPost, "http://"+addr+"/control/undrain", nil)
	req.Header.Set("X-Auth-Token", token)
	r, err = http.DefaultClient.Do(req)
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("undrain: %v %v", err, r.Status)
	}
	_ = r.Body.Close()
	if IsDraining() {
		t.Fatalf("expected not draining")
	}

	// Shutdown
	req, _ = http.NewRequest(http.MethodPost, "http://"+addr+"/control/shutdown", nil)
	req.Header.Set("X-Auth-Token", token)
	r, err = http.DefaultClient.Do(req)
	if err != nil || r.StatusCode != http.StatusOK {
		t.Fatalf("shutdown: %v %v", err, r.Status)
	}
	_ = r.Body.Close()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatalf("context not canceled")
	}
}
