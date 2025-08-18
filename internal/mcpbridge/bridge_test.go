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

func TestBridgeServerRequest(t *testing.T) {
	reqPayload := json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"foo"}`)
	respPayload := json.RawMessage(`{"jsonrpc":"2.0","id":1,"result":{}}`)
	streamPayload := json.RawMessage(`{"delta":1}`)
	recv := make(chan Frame, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		go func() {
			ctx := context.Background()
			// respond to initial dummy request
			_, data, _ := c.Read(ctx)
			var f Frame
			_ = json.Unmarshal(data, &f)
			resp := Frame{Type: TypeResponse, ID: f.ID, SessionID: f.SessionID, Payload: f.Payload}
			b, _ := json.Marshal(resp)
			_ = c.Write(ctx, websocket.MessageText, b)

			// send server_request
			sr := Frame{Type: TypeServerRequest, ID: "srv1", SessionID: f.SessionID, Payload: reqPayload}
			sb, _ := json.Marshal(sr)
			_ = c.Write(ctx, websocket.MessageText, sb)

			for i := 0; i < 2; i++ {
				_, data, err := c.Read(ctx)
				if err != nil {
					return
				}
				var rf Frame
				_ = json.Unmarshal(data, &rf)
				recv <- rf
			}
		}()
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	br := NewBridge(wsURL, 4)
	ctx := context.Background()
	// establish session via dummy request
	go func() {
		_, _ = br.Forward(ctx, "s1", json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), json.RawMessage(`1`), nil)
	}()
	sr := <-br.ServerRequests()
	if sr.ID != "srv1" || sr.SessionID != "s1" {
		t.Fatalf("unexpected server request: %+v", sr)
	}
	if err := br.ServerStream(ctx, sr.SessionID, sr.ID, streamPayload); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if err := br.ServerRespond(ctx, sr.SessionID, sr.ID, respPayload); err != nil {
		t.Fatalf("respond: %v", err)
	}
	se := <-recv
	if se.Type != TypeStreamEvent || se.ID != sr.ID || string(se.Payload) != string(streamPayload) {
		t.Fatalf("bad stream event: %+v", se)
	}
	rf := <-recv
	if rf.Type != TypeServerResponse || rf.ID != sr.ID || string(rf.Payload) != string(respPayload) {
		t.Fatalf("bad server response: %+v", rf)
	}
	br.Close()
}

func TestBridgeHealthy(t *testing.T) {
	br := NewBridge("ws://example", 1)
	if br.Healthy() {
		t.Fatalf("expected unhealthy")
	}
}
