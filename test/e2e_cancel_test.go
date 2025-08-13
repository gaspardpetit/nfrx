package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/relay"
	"github.com/you/llamapool/internal/server"
)

func TestCancelPropagates(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{WorkerKey: "secret", WSPath: "/workers/connect", RequestTimeout: 5 * time.Second}
	metricsReg := ctrl.NewMetricsRegistry("test", "", "")
	handler := server.New(reg, metricsReg, sched, cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/workers/connect"
	cancelReceived := make(chan struct{})
	go func() {
		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", WorkerKey: "secret", Models: []string{"m"}, MaxConcurrency: 1}
		b, _ := json.Marshal(regMsg)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			return
		}
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var env struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(data, &env); err != nil {
				continue
			}
			switch env.Type {
			case "job_request":
				var jr ctrl.JobRequestMessage
				if err := json.Unmarshal(data, &jr); err != nil {
					continue
				}
				chunk := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(`{"response":"hi","done":false}`)}
				bb, _ := json.Marshal(chunk)
				if err := conn.Write(ctx, websocket.MessageText, bb); err != nil {
					return
				}
			case "cancel_job":
				close(cancelReceived)
				return
			}
		}
	}()

	// wait for worker registration
	for i := 0; i < 50; i++ {
		if len(reg.Models()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	req := relay.GenerateRequest{Model: "m", Prompt: "hi", Stream: true}
	b, _ := json.Marshal(req)
	cctx, cancel := context.WithCancel(context.Background())
	httpReq, _ := http.NewRequestWithContext(cctx, http.MethodPost, srv.URL+"/api/generate", bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	scanner := bufio.NewScanner(resp.Body)
	if !scanner.Scan() {
		t.Fatalf("expected first line")
	}
	cancel()
	if scanner.Scan() {
		t.Fatalf("unexpected extra line after cancel")
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
	select {
	case <-cancelReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not receive cancel")
	}
}
