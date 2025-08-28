package mcpclient

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/coder/websocket"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/gaspardpetit/nfrx/modules/mcp/agent/internal/mcpbridge"
)

type fakeWSConn struct {
	writeCh chan []byte
}

func (f *fakeWSConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	return 0, nil, context.Canceled
}

func (f *fakeWSConn) Write(ctx context.Context, typ websocket.MessageType, data []byte) error {
	f.writeCh <- data
	return nil
}

type proxyTransport struct {
	lastReq transport.JSONRPCRequest
	resp    transport.JSONRPCResponse
	handler func(mcp.JSONRPCNotification)
	stream  []mcp.JSONRPCNotification
}

func (f *proxyTransport) Start(context.Context) error { return nil }
func (f *proxyTransport) SendRequest(ctx context.Context, req transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	f.lastReq = req
	for _, n := range f.stream {
		if f.handler != nil {
			f.handler(n)
		}
	}
	return &f.resp, nil
}
func (f *proxyTransport) SendNotification(context.Context, mcp.JSONRPCNotification) error { return nil }
func (f *proxyTransport) SetNotificationHandler(h func(mcp.JSONRPCNotification))          { f.handler = h }
func (f *proxyTransport) Close() error                                                    { return nil }
func (f *proxyTransport) GetSessionId() string                                            { return "" }

type bidirTransport struct {
	proxyTransport
	reqHandler transport.RequestHandler
}

func (b *bidirTransport) SetRequestHandler(h transport.RequestHandler) { b.reqHandler = h }

func TestProxyHandleRequest(t *testing.T) {
	reqJSON := []byte(`{"jsonrpc":"2.0","id":1,"method":"test","params":{"a":1}}`)
	resp := transport.JSONRPCResponse{JSONRPC: "2.0", ID: mcp.NewRequestId(1), Result: json.RawMessage(`{"ok":true}`)}
	ft := &proxyTransport{resp: resp}
	conn := newTransportConnector(ft, 0)
	ws := &fakeWSConn{writeCh: make(chan []byte, 1)}
	p := &Proxy{conn: ws, sessions: map[string]*sessionState{"sess": {conn: conn}}}

	frame := mcpbridge.Frame{Type: mcpbridge.TypeRequest, ID: "corr", SessionID: "sess", Payload: reqJSON}
	if err := p.handleFrame(context.Background(), frame); err != nil {
		t.Fatalf("handle: %v", err)
	}
	gotReqBytes, _ := json.Marshal(ft.lastReq)
	var gotReq, wantReq map[string]any
	_ = json.Unmarshal(gotReqBytes, &gotReq)
	_ = json.Unmarshal(reqJSON, &wantReq)
	if !reflect.DeepEqual(gotReq, wantReq) {
		t.Fatalf("request mismatch: %v vs %v", gotReq, wantReq)
	}
	outBytes := <-ws.writeCh
	var outFrame mcpbridge.Frame
	if err := json.Unmarshal(outBytes, &outFrame); err != nil {
		t.Fatalf("unmarshal frame: %v", err)
	}
	if outFrame.ID != frame.ID {
		t.Fatalf("corr id mismatch got %s want %s", outFrame.ID, frame.ID)
	}
	var gotResp, wantResp map[string]any
	_ = json.Unmarshal(outFrame.Payload, &gotResp)
	respBytes, _ := json.Marshal(resp)
	_ = json.Unmarshal(respBytes, &wantResp)
	if !reflect.DeepEqual(gotResp, wantResp) {
		t.Fatalf("response mismatch: %v vs %v", gotResp, wantResp)
	}
}

func TestProxyHandleStream(t *testing.T) {
	reqJSON := []byte(`{"jsonrpc":"2.0","id":1,"method":"test"}`)
	resp := transport.JSONRPCResponse{JSONRPC: "2.0", ID: mcp.NewRequestId(1)}
	stream := []mcp.JSONRPCNotification{
		{JSONRPC: "2.0", Notification: mcp.Notification{Method: "note", Params: mcp.NotificationParams{AdditionalFields: map[string]any{"i": 1}}}},
		{JSONRPC: "2.0", Notification: mcp.Notification{Method: "note", Params: mcp.NotificationParams{AdditionalFields: map[string]any{"i": 2}}}},
	}
	ft := &proxyTransport{resp: resp, stream: stream}
	conn := newTransportConnector(ft, 0)
	ws := &fakeWSConn{writeCh: make(chan []byte, 3)}
	p := &Proxy{conn: ws, sessions: map[string]*sessionState{"sess": {conn: conn}}}

	frame := mcpbridge.Frame{Type: mcpbridge.TypeRequest, ID: "corr", SessionID: "sess", Payload: reqJSON}
	if err := p.handleFrame(context.Background(), frame); err != nil {
		t.Fatalf("handle: %v", err)
	}
	for i := 0; i < 2; i++ {
		outBytes := <-ws.writeCh
		var f mcpbridge.Frame
		if err := json.Unmarshal(outBytes, &f); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if f.Type != mcpbridge.TypeStreamEvent || f.ID != "corr" {
			t.Fatalf("unexpected frame: %+v", f)
		}
	}
	respBytes := <-ws.writeCh
	var respFrame mcpbridge.Frame
	if err := json.Unmarshal(respBytes, &respFrame); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if respFrame.Type != mcpbridge.TypeResponse {
		t.Fatalf("expected response frame got %s", respFrame.Type)
	}
}

func TestProxyServerRequest(t *testing.T) {
	ws := &fakeWSConn{writeCh: make(chan []byte, 1)}
	bt := &bidirTransport{}
	conn := newTransportConnector(bt, 0)
	p := NewProxy(ws, func(ctx context.Context, id string) (Connector, error) { return conn, nil })
	if _, err := p.getSession(context.Background(), "sess"); err != nil {
		t.Fatalf("getSession: %v", err)
	}

	req := transport.JSONRPCRequest{JSONRPC: "2.0", ID: mcp.NewRequestId(1), Method: "ping"}
	if bt.reqHandler == nil {
		t.Fatalf("request handler not set")
	}
	var resp *transport.JSONRPCResponse
	done := make(chan struct{})
	go func() {
		resp, _ = bt.reqHandler(context.Background(), req)
		close(done)
	}()

	out := <-ws.writeCh
	var f mcpbridge.Frame
	if err := json.Unmarshal(out, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.Type != mcpbridge.TypeServerRequest {
		t.Fatalf("expected server_request got %s", f.Type)
	}
	respPayload := transport.JSONRPCResponse{JSONRPC: "2.0", ID: mcp.NewRequestId(1)}
	b, _ := json.Marshal(respPayload)
	frame := mcpbridge.Frame{Type: mcpbridge.TypeServerResponse, ID: f.ID, SessionID: "sess", Payload: b}
	if err := p.handleFrame(context.Background(), frame); err != nil {
		t.Fatalf("handleFrame: %v", err)
	}
	<-done
	if resp == nil {
		t.Fatalf("no response")
	}
}
