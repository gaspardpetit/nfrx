package worker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	ctrl "github.com/gaspardpetit/nfrx-sdk/contracts/control"
	aconfig "github.com/gaspardpetit/nfrx/modules/llm/agent/internal/config"
	"github.com/gaspardpetit/nfrx/modules/llm/agent/internal/relay"
)

func TestDrainAndTerminate(t *testing.T) {
	resetState()
	release := make(chan struct{})
	started := make(chan struct{})
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"m1"}]}`))
		case "/api/generate":
			close(started)
			<-release
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"done":true}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer ollama.Close()

	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		connCh <- c
	}))
	defer srv.Close()
	wsURL := "ws://" + srv.Listener.Addr().String()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	statusAddr := ln.Addr().String()
	_ = ln.Close()

	cfg := aconfig.WorkerConfig{ServerURL: wsURL, CompletionBaseURL: ollama.URL + "/v1", MaxConcurrency: 1, EmbeddingBatchSize: 0, StatusAddr: statusAddr, ConfigFile: filepath.Join(t.TempDir(), "worker.yaml")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, cfg) }()

	srvConn := <-connCh
	ctxR, cancelR := context.WithTimeout(context.Background(), time.Second)
	defer cancelR()
	if _, _, err := srvConn.Read(ctxR); err != nil { // register
		t.Fatalf("read register: %v", err)
	}

	jr1 := ctrl.JobRequestMessage{Type: "job_request", JobID: "j1", Endpoint: "generate", Payload: relay.GenerateRequest{Model: "m1", Prompt: "hi"}}
	b1, _ := json.Marshal(jr1)
	if err := srvConn.Write(context.Background(), websocket.MessageText, b1); err != nil {
		t.Fatalf("write j1: %v", err)
	}

	<-started
	StartDrain()

	jr2 := ctrl.JobRequestMessage{Type: "job_request", JobID: "j2", Endpoint: "generate", Payload: relay.GenerateRequest{Model: "m1", Prompt: "hi"}}
	b2, _ := json.Marshal(jr2)
	if err := srvConn.Write(context.Background(), websocket.MessageText, b2); err != nil {
		t.Fatalf("write j2: %v", err)
	}

	gotErr := false
	for !gotErr {
		_, msg, err := srvConn.Read(context.Background())
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg, &env); err != nil {
			continue
		}
		if env.Type == "job_error" {
			var je ctrl.JobErrorMessage
			_ = json.Unmarshal(msg, &je)
			if je.JobID == "j2" && je.Code == "worker_draining" {
				gotErr = true
			}
		}
	}

	resp, err := http.Get("http://" + statusAddr + "/status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var st State
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	_ = resp.Body.Close()
	if st.State != "draining" || st.CurrentJobs != 1 {
		t.Fatalf("unexpected status %+v", st)
	}

	close(release)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for worker exit")
	}
}

func TestDrainTerminatesWhenIdle(t *testing.T) {
	resetState()
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"m1"}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer ollama.Close()

	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		connCh <- c
	}))
	defer srv.Close()
	wsURL := "ws://" + srv.Listener.Addr().String()

	cfg := aconfig.WorkerConfig{ServerURL: wsURL, CompletionBaseURL: ollama.URL + "/v1", MaxConcurrency: 1, EmbeddingBatchSize: 0}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- Run(ctx, cfg) }()

	srvConn := <-connCh
	ctxR, cancelR := context.WithTimeout(context.Background(), time.Second)
	defer cancelR()
	if _, _, err := srvConn.Read(ctxR); err != nil {
		t.Fatalf("read register: %v", err)
	}

	StartDrain()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("run error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout waiting for worker exit")
	}
}
