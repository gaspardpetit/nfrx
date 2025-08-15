package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// Connector is a transport-agnostic MCP client connection.
type Connector interface {
	Start(ctx context.Context) error
	Initialize(ctx context.Context, req mcp.InitializeRequest) (*mcp.InitializeResult, error)
	DoRPC(ctx context.Context, method string, params any, result any) error
	Close() error
}

// transportConnector wraps a transport.Interface to satisfy Connector.
type transportConnector struct {
	t          transport.Interface
	id         atomic.Int64
	serverCaps mcp.ServerCapabilities
	protocol   string
}

func newTransportConnector(t transport.Interface) *transportConnector {
	return &transportConnector{t: t}
}

func (c *transportConnector) Start(ctx context.Context) error {
	return c.t.Start(ctx)
}

func (c *transportConnector) Close() error { return c.t.Close() }

func (c *transportConnector) Initialize(ctx context.Context, req mcp.InitializeRequest) (*mcp.InitializeResult, error) {
	params := struct {
		ProtocolVersion string                 `json:"protocolVersion"`
		ClientInfo      mcp.Implementation     `json:"clientInfo"`
		Capabilities    mcp.ClientCapabilities `json:"capabilities"`
	}{
		ProtocolVersion: req.Params.ProtocolVersion,
		ClientInfo:      req.Params.ClientInfo,
		Capabilities:    req.Params.Capabilities,
	}
	var res mcp.InitializeResult
	if err := c.DoRPC(ctx, string(mcp.MethodInitialize), params, &res); err != nil {
		return nil, err
	}
	if !slices.Contains(mcp.ValidProtocolVersions, res.ProtocolVersion) {
		return nil, mcp.UnsupportedProtocolVersionError{Version: res.ProtocolVersion}
	}
	c.serverCaps = res.Capabilities
	c.protocol = res.ProtocolVersion
	// best effort notification
	_ = c.t.SendNotification(ctx, mcp.JSONRPCNotification{JSONRPC: mcp.JSONRPC_VERSION, Notification: mcp.Notification{Method: "notifications/initialized"}})
	return &res, nil
}

func (c *transportConnector) DoRPC(ctx context.Context, method string, params any, result any) error {
	id := c.id.Add(1)
	req := transport.JSONRPCRequest{JSONRPC: mcp.JSONRPC_VERSION, ID: mcp.NewRequestId(id), Method: method, Params: params}
	resp, err := c.t.SendRequest(ctx, req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return errors.New(resp.Error.Message)
	}
	if result != nil && resp.Result != nil {
		return json.Unmarshal(resp.Result, result)
	}
	return nil
}

// Constructors for specific transports

func newStdioConnector(cfg Config) (*transportConnector, error) {
	if cfg.Stdio.Command == "" {
		return nil, fmt.Errorf("stdio command not configured")
	}
	t := transport.NewStdio(cfg.Stdio.Command, cfg.Stdio.Env, cfg.Stdio.Args...)
	return newTransportConnector(t), nil
}

func newHTTPConnector(cfg Config) (*transportConnector, error) {
	if cfg.HTTP.URL == "" {
		return nil, fmt.Errorf("http url not configured")
	}
	t, err := transport.NewStreamableHTTP(cfg.HTTP.URL)
	if err != nil {
		return nil, err
	}
	return newTransportConnector(t), nil
}

func newOAuthHTTPConnector(cfg Config) (*transportConnector, error) {
	if !cfg.OAuth.Enabled {
		return nil, fmt.Errorf("oauth disabled")
	}
	t, err := transport.NewStreamableHTTP(cfg.HTTP.URL, transport.WithHTTPOAuth(transport.OAuthConfig{
		ClientID:              cfg.OAuth.ClientID,
		ClientSecret:          cfg.OAuth.ClientSecret,
		Scopes:                cfg.OAuth.Scopes,
		AuthServerMetadataURL: cfg.OAuth.TokenURL,
	}))
	if err != nil {
		return nil, err
	}
	return newTransportConnector(t), nil
}

func newLegacySSEConnector(cfg Config) (*transportConnector, error) {
	if !cfg.EnableLegacySSE {
		return nil, fmt.Errorf("legacy sse disabled")
	}
	if cfg.HTTP.URL == "" {
		return nil, fmt.Errorf("http url not configured")
	}
	t, err := transport.NewSSE(cfg.HTTP.URL)
	if err != nil {
		return nil, err
	}
	return newTransportConnector(t), nil
}
