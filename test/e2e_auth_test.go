package test

import (
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
	"github.com/you/llamapool/internal/server"
)

func TestWorkerAuth(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{WorkerKey: "secret", WSPath: "/workers/connect", RequestTimeout: 5 * time.Second}
	handler := server.New(reg, sched, cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/workers/connect"

	// bad key
	connBad, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial bad: %v", err)
	}
	regBad := ctrl.RegisterMessage{Type: "register", WorkerID: "wbad", WorkerKey: "nope", Models: []string{"m"}, MaxConcurrency: 1}
	b, _ := json.Marshal(regBad)
	connBad.Write(ctx, websocket.MessageText, b)
	_, _, err = connBad.Read(ctx)
	if err == nil {
		t.Fatalf("expected close for bad key")
	}
	connBad.Close(websocket.StatusNormalClosure, "")
	if len(reg.Models()) != 0 {
		t.Fatalf("unexpected worker registered")
	}

	// good key
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: "w1", WorkerKey: "secret", Models: []string{"m"}, MaxConcurrency: 1}
	b, _ = json.Marshal(regMsg)
	conn.Write(ctx, websocket.MessageText, b)

	// wait for registration
	for i := 0; i < 50; i++ {
		if len(reg.Models()) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/tags", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("tags: %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()
}
