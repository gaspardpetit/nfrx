package mcpclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/gaspardpetit/llamapool/internal/logx"
)

// Connector is a transport-agnostic MCP client connection.
type Connector interface {
	Start(ctx context.Context) error
	Initialize(ctx context.Context, req mcp.InitializeRequest) (*mcp.InitializeResult, error)
	DoRPC(ctx context.Context, method string, params any, result any) error
	Capabilities() mcp.ServerCapabilities
	Protocol() string
	Features() Features
	Close() error
}

// transportConnector wraps a transport.Interface to satisfy Connector.
type transportConnector struct {
	t          transport.Interface
	id         atomic.Int64
	serverCaps mcp.ServerCapabilities
	protocol   string
	features   Features
	sem        chan struct{}
}

func newTransportConnector(t transport.Interface, maxInFlight int) *transportConnector {
	var sem chan struct{}
	if maxInFlight > 0 {
		sem = make(chan struct{}, maxInFlight)
	}
	return &transportConnector{t: t, sem: sem}
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
	c.features = deriveFeatures(res.Capabilities)
	// best effort notification
	_ = c.t.SendNotification(ctx, mcp.JSONRPCNotification{JSONRPC: mcp.JSONRPC_VERSION, Notification: mcp.Notification{Method: "notifications/initialized"}})
	return &res, nil
}

func (c *transportConnector) DoRPC(ctx context.Context, method string, params any, result any) error {
	if c.sem != nil {
		select {
		case c.sem <- struct{}{}:
			defer func() { <-c.sem }()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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

func (c *transportConnector) Capabilities() mcp.ServerCapabilities { return c.serverCaps }
func (c *transportConnector) Protocol() string                     { return c.protocol }
func (c *transportConnector) Features() Features                   { return c.features }

// Constructors for specific transports

func newStdioConnector(cfg Config) (*transportConnector, error) {
	if cfg.Stdio.Command == "" {
		return nil, fmt.Errorf("stdio command not configured")
	}
	if !filepath.IsAbs(cfg.Stdio.Command) {
		if !cfg.Stdio.AllowRelative {
			return nil, fmt.Errorf("stdio command must be absolute")
		}
		logx.Log.Warn().Str("command", cfg.Stdio.Command).Msg("allowing relative stdio command path")
	}
	env := buildEnv(cfg.Stdio.Env)
	t := transport.NewStdio(cfg.Stdio.Command, env, cfg.Stdio.Args...)
	return newTransportConnector(t, cfg.MaxInFlight), nil
}

func newHTTPConnector(cfg Config) (*transportConnector, error) {
	if cfg.HTTP.URL == "" {
		return nil, fmt.Errorf("http url not configured")
	}
	tlsCfg := &tls.Config{}
	if cfg.HTTP.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
		logx.Log.Warn().Str("url", cfg.HTTP.URL).Msg("TLS verification disabled")
	}
	client := &http.Client{Timeout: cfg.HTTP.Timeout, Transport: &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 10, IdleConnTimeout: 90 * time.Second, TLSClientConfig: tlsCfg}}
	opts := []transport.StreamableHTTPCOption{transport.WithHTTPBasicClient(client), transport.WithHTTPTimeout(cfg.HTTP.Timeout)}
	if cfg.HTTP.EnablePush {
		opts = append(opts, transport.WithContinuousListening())
	}
	t, err := transport.NewStreamableHTTP(cfg.HTTP.URL, opts...)
	if err != nil {
		return nil, err
	}
	return newTransportConnector(t, cfg.MaxInFlight), nil
}

func newOAuthHTTPConnector(cfg Config) (*transportConnector, error) {
	if !cfg.OAuth.Enabled {
		return nil, fmt.Errorf("oauth disabled")
	}
	if cfg.OAuth.TokenStore == nil && cfg.OAuth.TokenFile != "" {
		cfg.OAuth.TokenStore = NewFileTokenStore(cfg.OAuth.TokenFile)
	}
	tlsCfg := &tls.Config{}
	if cfg.HTTP.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
		logx.Log.Warn().Str("url", cfg.HTTP.URL).Msg("TLS verification disabled")
	}
	client := &http.Client{Timeout: cfg.HTTP.Timeout, Transport: &http.Transport{MaxIdleConns: 100, MaxIdleConnsPerHost: 10, IdleConnTimeout: 90 * time.Second, TLSClientConfig: tlsCfg}}
	opts := []transport.StreamableHTTPCOption{transport.WithHTTPBasicClient(client), transport.WithHTTPTimeout(cfg.HTTP.Timeout), transport.WithHTTPOAuth(transport.OAuthConfig{
		ClientID:              cfg.OAuth.ClientID,
		ClientSecret:          cfg.OAuth.ClientSecret,
		Scopes:                cfg.OAuth.Scopes,
		AuthServerMetadataURL: cfg.OAuth.TokenURL,
		TokenStore:            cfg.OAuth.TokenStore,
	})}
	if cfg.HTTP.EnablePush {
		opts = append(opts, transport.WithContinuousListening())
	}
	t, err := transport.NewStreamableHTTP(cfg.HTTP.URL, opts...)
	if err != nil {
		return nil, err
	}
	return newTransportConnector(t, cfg.MaxInFlight), nil
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
	return newTransportConnector(t, cfg.MaxInFlight), nil
}

// buildEnv constructs a limited environment from allowlisted variables.
// Each entry may be either "KEY" to copy from the current process env or
// "KEY=value" to set an explicit value. Variables not present in the current
// environment are skipped.
func buildEnv(vars []string) []string {
	var out []string
	for _, v := range vars {
		if strings.Contains(v, "=") {
			out = append(out, v)
			continue
		}
		if val, ok := os.LookupEnv(v); ok {
			out = append(out, fmt.Sprintf("%s=%s", v, val))
		}
	}
	return out
}

// Features describes server-supported capabilities for gating behavior.
type Features struct {
	Tools        bool
	Resources    bool
	Prompts      bool
	Logging      bool
	Sampling     bool
	Experimental map[string]any
}

func deriveFeatures(caps mcp.ServerCapabilities) Features {
	return Features{
		Tools:        caps.Tools != nil,
		Resources:    caps.Resources != nil,
		Prompts:      caps.Prompts != nil,
		Logging:      caps.Logging != nil,
		Sampling:     caps.Sampling != nil,
		Experimental: caps.Experimental,
	}
}
