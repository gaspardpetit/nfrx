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

func TestE2EGenerateStream(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{WorkerKey: "secret", WSPath: "/workers/connect", RequestTimeout: 5 * time.Second}
	handler := server.New(reg, sched, cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Fake worker
	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/workers/connect"
	go func() {
		conn, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", WorkerKey: "secret", Models: []string{"llama3"}, MaxConcurrency: 2}
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
			if env.Type == "job_request" {
				var jr ctrl.JobRequestMessage
				json.Unmarshal(data, &jr)
				chunk1 := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(`{"response":"hi","done":false}`)}
				b1, _ := json.Marshal(chunk1)
				conn.Write(ctx, websocket.MessageText, b1)
				chunk2 := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(`{"done":true}`)}
				b2, _ := json.Marshal(chunk2)
				conn.Write(ctx, websocket.MessageText, b2)
			}
		}
	}()

	// Wait for worker to register
	for i := 0; i < 20; i++ {
		resp, err := http.Get(srv.URL + "/api/tags")
		if err == nil {
			var tr struct {
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}
			json.NewDecoder(resp.Body).Decode(&tr)
			resp.Body.Close()
			if len(tr.Models) > 0 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Call generate
	req := relay.GenerateRequest{Model: "llama3", Prompt: "hi", Stream: true}
	b, _ := json.Marshal(req)
	resp, err := http.Post(srv.URL+"/api/generate", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[1], "\"done\":true") {
		t.Fatalf("missing done line")
	}
}
