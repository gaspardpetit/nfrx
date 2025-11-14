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
	"github.com/gaspardpetit/nfrx/sdk/base/agent"
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

	return agent.RunWithReconnect(ctx, cfg.Reconnect, func(runCtx context.Context) error {
		// Send client key as Authorization bearer header when present (for proxy auth)
		var dialOpts *websocket.DialOptions
		hasAuth := false
		if cfg.ClientKey != "" {
			hdr := make(http.Header)
			hdr.Set("Authorization", "Bearer "+cfg.ClientKey)
			dialOpts = &websocket.DialOptions{HTTPHeader: hdr}
			hasAuth = true
		}
		logx.Log.Info().Str("server", cfg.ServerURL).Bool("auth_header", hasAuth).Msg("dialing server")
		dctx, cancelDial := context.WithTimeout(runCtx, 15*time.Second)
		defer cancelDial()
		conn, resp, err := websocket.Dial(dctx, cfg.ServerURL, dialOpts)
		if err != nil {
			if resp != nil {
				logx.Log.Warn().Err(err).Str("server", cfg.ServerURL).Int("status", resp.StatusCode).Interface("headers", resp.Header).Msg("websocket dial failed")
			} else {
				logx.Log.Warn().Err(err).Str("server", cfg.ServerURL).Msg("websocket dial failed")
			}
			return err
		}

		reg := map[string]string{"id": cfg.ClientID, "client_name": cfg.ClientName}
		b, _ := json.Marshal(reg)
		if err := conn.Write(runCtx, websocket.MessageText, b); err != nil {
			_ = conn.Close(websocket.StatusInternalError, "closing")
			return err
		}

		_, msg, err := conn.Read(runCtx)
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "closing")
			return err
		}
		var ack mcpcommon.Ack
		_ = json.Unmarshal(msg, &ack)
		cfg.ClientID = ack.ID
		logx.Log.Info().Str("server", cfg.ServerURL).Str("client_id", cfg.ClientID).Str("client_name", cfg.ClientName).Msg("connected to server")

		childCtx, cancel := context.WithCancel(runCtx)
		defer cancel()
		streamPref := newStreamPref(cfg.AllowHTTPStreaming)
		go monitorProvider(childCtx, cfg.ProviderURL, cfg.Reconnect, streamPref)

		relay := newRelayClientWithPref(conn, cfg.ProviderURL, cfg.AuthToken, cfg.RequestTimeout, streamPref)
		if err := relay.Run(childCtx); err != nil {
			_ = conn.Close(websocket.StatusInternalError, "closing")
			return err
		}
		_ = conn.Close(websocket.StatusNormalClosure, "closing")
		return nil
	})
}

func monitorProvider(ctx context.Context, url string, shouldReconnect bool, pref *streamPref) {
	attempt := 0
	streaming := pref.Allow()
	for {
		logx.Log.Debug().Int("attempt", attempt).Str("url", url).Msg("checking mcp provider")
		// Apply a finite timeout to each probe
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := ProbeProvider(pctx, url, streaming)
		cancel()
		if err != nil {
			if isNotAcceptable(err) && streaming {
				logx.Log.Warn().Msg("provider rejected streaming; retrying without SSE")
				streaming = false
				pref.Set(false)
				attempt = 0
				continue
			}
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
		if pref.Allow() != streaming {
			pref.Set(streaming)
		}
		logx.Log.Info().Bool("streaming", streaming).Msg("mcp provider ready")
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
func ProbeProvider(ctx context.Context, url string, allowStreaming bool) error {
	logx.Log.Debug().Str("url", url).Bool("streaming", allowStreaming).Msg("probing mcp provider")
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
	if allowStreaming {
		req.Header.Set("Accept", "application/json, text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	logx.Log.Debug().Str("status", resp.Status).Str("body", summarizeBody(respBody, 512)).Msg("probe response")
	if resp.StatusCode >= http.StatusBadRequest {
		msg := strings.TrimSpace(string(respBody))
		if msg != "" {
			return fmt.Errorf("status %s: %s", resp.Status, msg)
		}
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}

func isNotAcceptable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "406") || strings.Contains(strings.ToLower(err.Error()), "not acceptable")
}
