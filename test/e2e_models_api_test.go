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

func TestModelsAPI(t *testing.T) {
	reg := ctrl.NewRegistry()
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	cfg := config.ServerConfig{WSPath: "/workers/connect", RequestTimeout: 5 * time.Second}
	handler := server.New(reg, sched, cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1) + "/workers/connect"

	// Worker Alpha
	connA, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial alpha: %v", err)
	}
	rmA := ctrl.RegisterMessage{Type: "register", WorkerID: "wA", WorkerName: "Alpha", Models: []string{"llama3:8b", "mistral:7b"}, MaxConcurrency: 1}
	b, _ := json.Marshal(rmA)
	connA.Write(ctx, websocket.MessageText, b)

	// Worker Beta
	connB, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial beta: %v", err)
	}
	rmB := ctrl.RegisterMessage{Type: "register", WorkerID: "wB", WorkerName: "Beta", Models: []string{"llama3:8b", "qwen2.5:14b"}, MaxConcurrency: 1}
	b, _ = json.Marshal(rmB)
	connB.Write(ctx, websocket.MessageText, b)

	// wait for registration
	for i := 0; i < 50; i++ {
		resp, err := http.Get(srv.URL + "/v1/models")
		if err == nil {
			var lr struct {
				Data []struct {
					ID      string `json:"id"`
					OwnedBy string `json:"owned_by"`
				} `json:"data"`
			}
			json.NewDecoder(resp.Body).Decode(&lr)
			resp.Body.Close()
			if len(lr.Data) == 3 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	var list struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Data) != 3 {
		t.Fatalf("expected 3 models, got %d", len(list.Data))
	}
	for _, m := range list.Data {
		if m.ID == "llama3:8b" && m.OwnedBy != "Alpha,Beta" {
			t.Fatalf("owned_by wrong: %s", m.OwnedBy)
		}
	}

	resp, err = http.Get(srv.URL + "/v1/models/llama3:8b")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("get model: %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()
	resp, err = http.Get(srv.URL + "/v1/models/doesnotexist")
	if err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing model: %v %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	connB.Close(websocket.StatusNormalClosure, "")
	// wait for deregistration
	for i := 0; i < 50; i++ {
		resp, err := http.Get(srv.URL + "/v1/models")
		if err == nil {
			var lr struct {
				Data []struct {
					ID      string `json:"id"`
					OwnedBy string `json:"owned_by"`
				} `json:"data"`
			}
			json.NewDecoder(resp.Body).Decode(&lr)
			resp.Body.Close()
			if len(lr.Data) == 2 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	resp, err = http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("list after disconnect: %v", err)
	}
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	for _, m := range list.Data {
		if m.ID == "llama3:8b" && m.OwnedBy != "Alpha" {
			t.Fatalf("owned_by after disconnect: %s", m.OwnedBy)
		}
		if m.ID == "qwen2.5:14b" {
			t.Fatalf("beta model still listed")
		}
	}

	connA.Close(websocket.StatusNormalClosure, "")
}
