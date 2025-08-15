package mcpclient

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

type fakeInitTransport struct{ res mcp.InitializeResult }

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
func (f *fakeInitTransport) GetSessionId() string                                 { return "" }

func TestFeatureDerivation(t *testing.T) {
	caps := mcp.ServerCapabilities{
		Tools: &struct {
			ListChanged bool `json:"listChanged,omitempty"`
		}{},
		Experimental: map[string]any{"progress": true},
	}
	ft := &fakeInitTransport{res: mcp.InitializeResult{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, Capabilities: caps}}
	conn := newTransportConnector(ft, 0)
	req := mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION}}
	if _, err := conn.Initialize(context.Background(), req); err != nil {
		t.Fatalf("initialize: %v", err)
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
