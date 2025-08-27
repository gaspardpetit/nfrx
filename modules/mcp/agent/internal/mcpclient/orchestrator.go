package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/rs/zerolog"

	"github.com/gaspardpetit/nfrx/modules/common/logx"
)

// Orchestrator attempts connections using multiple transports until one succeeds.
type Orchestrator struct {
	cfg       Config
	log       zerolog.Logger
	factories map[string]func(Config) (*transportConnector, error)
}

// NewOrchestrator constructs an orchestrator with default transport factories.
func NewOrchestrator(cfg Config) *Orchestrator {
	o := &Orchestrator{cfg: cfg, log: logx.Log, factories: map[string]func(Config) (*transportConnector, error){
		"stdio":      newStdioConnector,
		"http":       newHTTPConnector,
		"oauth":      newOAuthHTTPConnector,
		"legacy-sse": newLegacySSEConnector,
	}}
	return o
}

// Connect tries transports in order until one initializes successfully.
func (o *Orchestrator) Connect(ctx context.Context) (Connector, error) {
	var errs []error
	timeout := o.cfg.InitTimeout
	for _, name := range o.cfg.Order {
		factory, ok := o.factories[name]
		if !ok {
			continue
		}
		conn, err := factory(o.cfg)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s setup: %w", name, err))
			continue
		}
		ctxAttempt, cancel := context.WithTimeout(ctx, timeout)
		o.log.Info().Str("transport", name).Msg("connecting")
		if err := conn.Start(ctxAttempt); err != nil {
			cancel()
			errs = append(errs, fmt.Errorf("%s start: %w", name, err))
			_ = conn.Close()
			timeout = backoff(timeout)
			continue
		}
		version := o.cfg.ProtocolVersion
		if version == "" {
			version = mcp.LATEST_PROTOCOL_VERSION
		}
		initReq := mcp.InitializeRequest{
			Request: mcp.Request{Method: string(mcp.MethodInitialize)},
			Params: mcp.InitializeParams{
				ProtocolVersion: version,
				Capabilities:    mcp.ClientCapabilities{},
				ClientInfo:      mcp.Implementation{Name: "nfrx-mcp", Version: "dev"},
			},
		}
		if _, err := conn.Initialize(ctxAttempt, initReq); err != nil {
			cancel()
			errs = append(errs, fmt.Errorf("%s initialize: %w", name, err))
			_ = conn.Close()
			timeout = backoff(timeout)
			continue
		}
		cancel()
		if conn.Protocol() != version {
			o.log.Warn().Str("transport", name).Str("server_protocol", conn.Protocol()).Str("client_protocol", version).Msg("protocol downgraded")
		}
		o.log.Info().
			Str("transport", name).
			Str("session", conn.SessionID()).
			Str("protocol", conn.Protocol()).
			Interface("capabilities", conn.Capabilities()).
			Msg("connected")
		return conn, nil
	}
	return nil, errors.Join(errs...)
}

func backoff(d time.Duration) time.Duration {
	nd := d * 2
	if nd > 30*time.Second {
		return 30 * time.Second
	}
	return nd
}
