package workerproxy

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
)

func startWSServer(t *testing.T, handler func(*websocket.Conn)) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		handler(conn)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	url := "ws://" + ln.Addr().String()
	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = ln.Close()
	}
	return url, cleanup
}

func TestConnectAndServeAcceptsLegacyServerWithoutRegisterAck(t *testing.T) {
	statusSeen := make(chan ctrl.StatusUpdateMessage, 1)
	url, cleanup := startWSServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		ctx := context.Background()
		_, msg, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read register: %v", err)
		}
		var reg ctrl.RegisterMessage
		if err := json.Unmarshal(msg, &reg); err != nil {
			t.Fatalf("decode register: %v", err)
		}
		if reg.Type != "register" || reg.WorkerID != "legacy-worker" {
			t.Fatalf("unexpected register: %+v", reg)
		}
		su := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 1, Status: "idle"}
		b, _ := json.Marshal(su)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatalf("write legacy first message: %v", err)
		}
		_, next, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var seen ctrl.StatusUpdateMessage
		if err := json.Unmarshal(next, &seen); err == nil && seen.Type == "status_update" {
			statusSeen <- seen
		}
	})
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	SetConnectedToServer(false)
	SetState("connecting")
	SetWorkerInfo("legacy-worker", "Legacy", 1)
	SetLabels([]string{"m"})

	cfg := Config{ServerURL: url, ClientID: "legacy-worker", ClientName: "Legacy"}
	statusUpdates := make(chan ctrl.StatusUpdateMessage, 1)
	statusUpdates <- ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 1, Status: "idle"}

	done := make(chan error, 1)
	go func() {
		_, err := connectAndServe(ctx, cancel, cfg, statusUpdates)
		done <- err
	}()

	select {
	case seen := <-statusSeen:
		if seen.Type != "status_update" || seen.Status != "idle" {
			t.Fatalf("unexpected forwarded status update: %+v", seen)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("did not observe forwarded status update")
	}

	if st := GetState(); !st.ConnectedToServer || (st.State != "connected_idle" && st.State != "connected_busy") {
		t.Fatalf("expected connected state after legacy handshake, got %+v", st)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("connectAndServe did not exit after cancel")
	}
}
