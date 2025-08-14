package ctrl

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRegisterStoresWorkerName(t *testing.T) {
	reg := NewRegistry()
	metricsReg := NewMetricsRegistry("test", "", "")
	srv := httptest.NewServer(WSHandler(reg, metricsReg, ""))
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	rm := RegisterMessage{Type: "register", WorkerID: "w1abcdef", WorkerName: "Alpha", Models: []string{"m"}, MaxConcurrency: 1}
	b, _ := json.Marshal(rm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
	// wait for registration
	for i := 0; i < 50; i++ {
		reg.mu.RLock()
		w, ok := reg.workers["w1abcdef"]
		reg.mu.RUnlock()
		if ok {
			if w.Name != "Alpha" {
				t.Fatalf("expected name Alpha, got %s", w.Name)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRegisterFallbackName(t *testing.T) {
	reg := NewRegistry()
	metricsReg := NewMetricsRegistry("test", "", "")
	srv := httptest.NewServer(WSHandler(reg, metricsReg, ""))
	defer srv.Close()

	ctx := context.Background()
	wsURL := strings.Replace(srv.URL, "http", "ws", 1)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()
	rm := RegisterMessage{Type: "register", WorkerID: "w123456789", Models: []string{"m"}, MaxConcurrency: 1}
	b, _ := json.Marshal(rm)
	if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
	var name string
	for i := 0; i < 50; i++ {
		reg.mu.RLock()
		w, ok := reg.workers["w123456789"]
		reg.mu.RUnlock()
		if ok {
			name = w.Name
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if name == "" || name != "w1234567" {
		t.Fatalf("unexpected fallback name %q", name)
	}
}
