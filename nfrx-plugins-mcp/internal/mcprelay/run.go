package mcprelay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	agent "github.com/gaspardpetit/nfrx-plugins-mcp/internal/agent"
	"github.com/gaspardpetit/nfrx-sdk/config"
	"github.com/gaspardpetit/nfrx-sdk/ctrl"
	"github.com/gaspardpetit/nfrx-sdk/logx"
	reconnect "github.com/gaspardpetit/nfrx-sdk/reconnect"
)

// Run starts the MCP relay client and blocks until the context is canceled or a
// non-recoverable error occurs. It manages connection retries, provider
// availability checks, and optional metrics.
func Run(ctx context.Context, cfg config.MCPConfig) error {
	if cfg.ClientID == "" {
		cfg.ClientID = uuid.NewString()
	}
	if cfg.MetricsAddr != "" {
		if _, err := StartMetricsServer(ctx, cfg.MetricsAddr); err != nil {
			return err
		}
		logx.Log.Info().Str("addr", cfg.MetricsAddr).Msg("metrics server started")
	}

	u, err := url.Parse(cfg.ServerURL)
	if err != nil {
		return err
	}
	grpcAddr := cfg.ControlGRPCAddr
	if cfg.ControlGRPCSocket != "" {
		grpcAddr = "unix://" + cfg.ControlGRPCSocket
	}
	if grpcAddr == "" {
		host := u.Hostname()
		port, _ := strconv.Atoi(u.Port())
		grpcAddr = host + ":" + strconv.Itoa(port+1)
	}
	agentClient, err := agent.Dial(ctx, grpcAddr)
	if err != nil {
		return err
	}
	defer func() { _ = agentClient.Close() }()
	req := &ctrl.RegisterRequest{
		WorkerId:        cfg.ClientID,
		ProtocolVersion: ctrl.ProtocolVersion,
		Capabilities:    []string{"mcp"},
		Metadata: map[string]string{
			"worker_name": cfg.ClientName,
			"client_key":  cfg.ClientKey,
		},
	}
	if err := agentClient.Register(ctx, req, 5*time.Second); err != nil {
		return err
	}
	if err := agentClient.StartHeartbeat(ctx, cfg.ClientID, 5*time.Second); err != nil {
		return err
	}

	attempt := 0
	for {
		wsURL := *u
		q := wsURL.Query()
		q.Set("id", cfg.ClientID)
		if cfg.ClientKey != "" {
			q.Set("key", cfg.ClientKey)
		}
		wsURL.RawQuery = q.Encode()
		conn, _, err := websocket.Dial(ctx, wsURL.String(), nil)
		if err != nil {
			if !cfg.Reconnect {
				return err
			}
			delay := reconnect.Delay(attempt)
			attempt++
			logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("dial broker failed; retrying")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}
		logx.Log.Info().Str("server", cfg.ServerURL).Str("client_id", cfg.ClientID).Str("client_name", cfg.ClientName).Msg("connected to server")
		attempt = 0

		runCtx, cancel := context.WithCancel(ctx)
		go monitorProvider(runCtx, cfg.ProviderURL, cfg.Reconnect)

		relay := NewRelayClient(conn, cfg.ProviderURL, cfg.AuthToken, cfg.RequestTimeout)
		if err := relay.Run(runCtx); err != nil {
			cancel()
			_ = conn.Close(websocket.StatusInternalError, "closing")
			if !cfg.Reconnect {
				return err
			}
			delay := reconnect.Delay(attempt)
			attempt++
			logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("relay stopped; reconnecting")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}
		cancel()
		_ = conn.Close(websocket.StatusNormalClosure, "closing")
		return nil
	}
}

func monitorProvider(ctx context.Context, url string, shouldReconnect bool) {
	attempt := 0
	for {
		logx.Log.Debug().Int("attempt", attempt).Str("url", url).Msg("checking mcp provider")
		err := ProbeProvider(ctx, url)
		if err != nil {
			lvl := logx.Log.Warn()
			if strings.Contains(err.Error(), "status 401") || strings.Contains(err.Error(), "status 403") {
				lvl = logx.Log.Error()
			}
			lvl.Err(err).Msg("mcp provider unavailable; not_ready")
			if !shouldReconnect {
				return
			}
			delay := reconnect.Delay(attempt)
			attempt++
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}
		logx.Log.Info().Msg("mcp provider ready")
		attempt = 0
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
	}
}

// ProbeProvider checks if the MCP provider at the given URL responds to a basic
// tools/list request.
func ProbeProvider(ctx context.Context, url string) error {
	logx.Log.Debug().Str("url", url).Msg("probing mcp provider")
	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "tools/list",
		"params":  map[string]any{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	logx.Log.Debug().Str("status", resp.Status).Msg("probe response")
	if resp.StatusCode >= http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(b))
		if msg != "" {
			return fmt.Errorf("status %s: %s", resp.Status, msg)
		}
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}
