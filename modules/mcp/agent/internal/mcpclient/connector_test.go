package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	reconnect "github.com/gaspardpetit/nfrx/core/reconnect"
)

type fakeInitTransport struct {
	res       mcp.InitializeResult
	sessionID string
}

func (f *fakeInitTransport) Start(context.Context) error { return nil }
func (f *fakeInitTransport) SendRequest(ctx context.Context, req transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	if req.Method == string(mcp.MethodInitialize) {
		b, _ := json.Marshal(f.res)
		return &transport.JSONRPCResponse{Result: b}, nil
	}
	return &transport.JSONRPCResponse{Result: json.RawMessage(`{}`)}, nil
}
func (f *fakeInitTransport) SendNotification(context.Context, mcp.JSONRPCNotification) error {
	return nil
}
func (f *fakeInitTransport) SetNotificationHandler(func(mcp.JSONRPCNotification)) {}
func (f *fakeInitTransport) Close() error                                         { return nil }
func (f *fakeInitTransport) GetSessionId() string                                 { return f.sessionID }

func TestFeatureDerivation(t *testing.T) {
	caps := mcp.ServerCapabilities{
		Tools: &struct {
			ListChanged bool `json:"listChanged,omitempty"`
		}{},
		Experimental: map[string]any{"progress": true},
	}
	ft := &fakeInitTransport{res: mcp.InitializeResult{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, Capabilities: caps}, sessionID: "sess"}
	conn := newTransportConnector(ft, 0)
	req := mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION}}
	if _, err := conn.Initialize(context.Background(), req); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if conn.SessionID() != "sess" {
		t.Fatalf("session id not captured")
	}
	feats := conn.Features()
	if !feats.Tools {
		t.Fatalf("expected tools feature")
	}
	if feats.Resources {
		t.Fatalf("unexpected resources feature")
	}
	if _, ok := feats.Experimental["progress"]; !ok {
		t.Fatalf("experimental progress not recorded")
	}
}

func TestStdioCommandSecurity(t *testing.T) {
	cfg := Config{Stdio: StdioConfig{Command: "relative"}}
	if _, err := newStdioConnector(cfg); err == nil {
		t.Fatalf("expected error for relative command")
	}
	cfg.Stdio.AllowRelative = true
	if _, err := newStdioConnector(cfg); err != nil {
		t.Fatalf("allow relative: %v", err)
	}
}

func TestBuildEnv(t *testing.T) {
	t.Setenv("FOO", "bar")
	env := buildEnv([]string{"FOO", "BAR=baz", "MISSING"})
	want := map[string]string{"FOO": "bar", "BAR": "baz"}
	if len(env) != 2 {
		t.Fatalf("got %d env vars", len(env))
	}
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if want[parts[0]] != parts[1] {
			t.Fatalf("unexpected %s", kv)
		}
	}
}

type fakeCLTransport struct {
	startCount atomic.Int64
	handler    func(error)
}

func (f *fakeCLTransport) Start(context.Context) error {
	f.startCount.Add(1)
	return nil
}

func (f *fakeCLTransport) SendRequest(ctx context.Context, req transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	return &transport.JSONRPCResponse{Result: json.RawMessage(`{}`)}, nil
}

func (f *fakeCLTransport) SendNotification(context.Context, mcp.JSONRPCNotification) error {
	return nil
}
func (f *fakeCLTransport) SetNotificationHandler(func(mcp.JSONRPCNotification)) {}
func (f *fakeCLTransport) Close() error                                         { return nil }
func (f *fakeCLTransport) GetSessionId() string                                 { return "" }
func (f *fakeCLTransport) SetConnectionLostHandler(h func(error))               { f.handler = h }

func TestOnConnectionLostRestart(t *testing.T) {
	prev := reconnect.Schedule
	reconnect.Schedule = []time.Duration{time.Millisecond}
	defer func() { reconnect.Schedule = prev }()
	ft := &fakeCLTransport{}
	conn := newTransportConnector(ft, 0)
	if err := conn.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if ft.handler == nil {
		t.Fatalf("handler not set")
	}
	ft.handler(errors.New("lost"))
	deadline := time.After(time.Second)
	for {
		if ft.startCount.Load() >= 2 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("reconnect not triggered")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

type fakeRetryTransport struct {
	startCount atomic.Int32
	handler    func(error)
}

func (f *fakeRetryTransport) Start(context.Context) error {
	n := f.startCount.Add(1)
	if n == 1 {
		return nil
	}
	if n <= 3 {
		return errors.New("fail")
	}
	return nil
}

func (f *fakeRetryTransport) SendRequest(context.Context, transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	return nil, nil
}

func (f *fakeRetryTransport) SendNotification(context.Context, mcp.JSONRPCNotification) error {
	return nil
}
func (f *fakeRetryTransport) SetNotificationHandler(func(mcp.JSONRPCNotification)) {}
func (f *fakeRetryTransport) Close() error                                         { return nil }
func (f *fakeRetryTransport) GetSessionId() string                                 { return "" }
func (f *fakeRetryTransport) SetConnectionLostHandler(h func(error))               { f.handler = h }

func TestReconnectBackoff(t *testing.T) {
	prev := reconnect.Schedule
	reconnect.Schedule = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	defer func() { reconnect.Schedule = prev }()
	ft := &fakeRetryTransport{}
	conn := newTransportConnector(ft, 0)
	if err := conn.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if ft.handler == nil {
		t.Fatalf("handler not set")
	}
	ft.handler(errors.New("lost"))
	time.Sleep(10 * time.Millisecond)
	if ft.startCount.Load() < 4 {
		t.Fatalf("expected multiple restart attempts, got %d", ft.startCount.Load())
	}
}
