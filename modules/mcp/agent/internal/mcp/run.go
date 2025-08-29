package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

    "github.com/gaspardpetit/nfrx/core/logx"
    reconnect "github.com/gaspardpetit/nfrx/core/reconnect"
    aconfig "github.com/gaspardpetit/nfrx/modules/mcp/agent/internal/config"
    mcpcommon "github.com/gaspardpetit/nfrx/modules/mcp/common"
)

// Run starts the MCP relay client and blocks until the context is canceled or a
// non-recoverable error occurs. It manages connection retries, provider
// availability checks, and optional metrics.
func Run(ctx context.Context, cfg aconfig.MCPConfig) error {
	if cfg.MetricsAddr != "" {
		if _, err := StartMetricsServer(ctx, cfg.MetricsAddr); err != nil {
			return err
		}
		logx.Log.Info().Str("addr", cfg.MetricsAddr).Msg("metrics server started")
	}

	attempt := 0
	for {
		conn, _, err := websocket.Dial(ctx, cfg.ServerURL, nil)
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

		reg := map[string]string{"id": cfg.ClientID, "client_name": cfg.ClientName}
		if cfg.ClientKey != "" {
			reg["client_key"] = cfg.ClientKey
		}
		b, _ := json.Marshal(reg)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			_ = conn.Close(websocket.StatusInternalError, "closing")
			if !cfg.Reconnect {
				return err
			}
			delay := reconnect.Delay(attempt)
			attempt++
			logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("register failed; retrying")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}

		_, msg, err := conn.Read(ctx)
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "closing")
			if !cfg.Reconnect {
				return err
			}
			delay := reconnect.Delay(attempt)
			attempt++
			logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("register ack failed; retrying")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}
        var ack mcpcommon.Ack
        _ = json.Unmarshal(msg, &ack)
        cfg.ClientID = ack.ID
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
