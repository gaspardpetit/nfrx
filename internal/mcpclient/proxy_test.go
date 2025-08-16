package mcpclient

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/coder/websocket"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/gaspardpetit/llamapool/internal/mcpbridge"
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
}

func (f *proxyTransport) Start(context.Context) error { return nil }
func (f *proxyTransport) SendRequest(ctx context.Context, req transport.JSONRPCRequest) (*transport.JSONRPCResponse, error) {
	f.lastReq = req
	return &f.resp, nil
}
func (f *proxyTransport) SendNotification(context.Context, mcp.JSONRPCNotification) error { return nil }
func (f *proxyTransport) SetNotificationHandler(func(mcp.JSONRPCNotification))            {}
func (f *proxyTransport) Close() error                                                    { return nil }
func (f *proxyTransport) GetSessionId() string                                            { return "" }

func TestProxyHandleRequest(t *testing.T) {
	reqJSON := []byte(`{"jsonrpc":"2.0","id":1,"method":"test","params":{"a":1}}`)
	resp := transport.JSONRPCResponse{JSONRPC: "2.0", ID: mcp.NewRequestId(1), Result: json.RawMessage(`{"ok":true}`)}
	ft := &proxyTransport{resp: resp}
	conn := newTransportConnector(ft, 0)
	ws := &fakeWSConn{writeCh: make(chan []byte, 1)}
	p := &Proxy{conn: ws, sessions: map[string]Connector{"sess": conn}}

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
