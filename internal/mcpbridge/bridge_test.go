package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestBridgeRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		go func() {
			ctx := context.Background()
			for {
				_, data, err := c.Read(ctx)
				if err != nil {
					return
				}
				var f Frame
				if json.Unmarshal(data, &f) != nil {
					continue
				}
				if f.Type != TypeRequest {
					continue
				}
				resp := Frame{Type: TypeResponse, ID: f.ID, SessionID: f.SessionID, Payload: f.Payload}
				b, _ := json.Marshal(resp)
				_ = c.Write(ctx, websocket.MessageText, b)
			}
		}()
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	br := NewBridge(wsURL, 4)
	ctx := context.Background()
	payload := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	resp, err := br.Forward(ctx, "s1", payload, json.RawMessage(`1`), nil)
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if string(resp) != string(payload) {
		t.Fatalf("payload mismatch: %s != %s", resp, payload)
	}
	br.Close()
}

func TestBridgeBackpressure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		// keep connection open but do not respond
		_ = c
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	br := NewBridge(wsURL, 1)
	ctx := context.Background()
	payload := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	ctx1, cancel1 := context.WithTimeout(ctx, time.Second)
	go func() { _, _ = br.Forward(ctx1, "s1", payload, json.RawMessage(`1`), nil) }()
	// wait for first request to register
	time.Sleep(100 * time.Millisecond)
	_, err := br.Forward(ctx, "s1", payload, json.RawMessage(`1`), nil)
	if !errors.Is(err, ErrBackpressure) {
		t.Fatalf("expected ErrBackpressure got %v", err)
	}
	cancel1()
	br.Close()
}

func TestBridgeStreamEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		go func() {
			ctx := context.Background()
			for {
				_, data, err := c.Read(ctx)
				if err != nil {
					return
				}
				var f Frame
				if json.Unmarshal(data, &f) != nil {
					continue
				}
				if f.Type != TypeRequest {
					continue
				}
				for i := 0; i < 2; i++ {
					se := Frame{Type: TypeStreamEvent, ID: f.ID, SessionID: f.SessionID, Payload: json.RawMessage([]byte(fmt.Sprintf("{\"i\":%d}", i)))}
					b, _ := json.Marshal(se)
					_ = c.Write(ctx, websocket.MessageText, b)
				}
				resp := Frame{Type: TypeResponse, ID: f.ID, SessionID: f.SessionID, Payload: f.Payload}
				b, _ := json.Marshal(resp)
				_ = c.Write(ctx, websocket.MessageText, b)
			}
		}()
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	br := NewBridge(wsURL, 4)
	ctx := context.Background()
	payload := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	var events []string
	stream := func(p json.RawMessage) { events = append(events, string(p)) }
	if _, err := br.Forward(ctx, "s1", payload, json.RawMessage(`1`), stream); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events got %d", len(events))
	}
	if events[0] != `{"i":0}` || events[1] != `{"i":1}` {
		t.Fatalf("unexpected events: %v", events)
	}
	br.Close()
}
