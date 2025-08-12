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

	"nhooyr.io/websocket"

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/relay"
	"github.com/you/llamapool/internal/server"
)

func TestCancelPropagates(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{WorkerKey: "secret", WSPath: "/workers/connect", RequestTimeout: 5 * time.Second}
	handler := server.New(reg, sched, cfg)
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
		defer conn.Close(websocket.StatusNormalClosure, "")
		regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", WorkerKey: "secret", Models: []string{"m"}, MaxConcurrency: 1}
		b, _ := json.Marshal(regMsg)
		conn.Write(ctx, websocket.MessageText, b)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var env struct {
				Type string `json:"type"`
			}
			json.Unmarshal(data, &env)
			switch env.Type {
			case "job_request":
				var jr ctrl.JobRequestMessage
				json.Unmarshal(data, &jr)
				chunk := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(`{"response":"hi","done":false}`)}
				bb, _ := json.Marshal(chunk)
				conn.Write(ctx, websocket.MessageText, bb)
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
	resp.Body.Close()
	select {
	case <-cancelReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not receive cancel")
	}
}
