package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/gaspardpetit/llamapool/internal/agent"
	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/logx"
	reconnect "github.com/gaspardpetit/llamapool/internal/reconnect"
)

// Run connects to the server and relays MCP requests to the provider.
func Run(ctx context.Context, cfg config.MCPRelayConfig) error {
	connect := func(ctx context.Context) (bool, error) {
		conn, _, err := websocket.Dial(ctx, cfg.ServerURL, nil)
		if err != nil {
			return false, err
		}
		defer func() {
			_ = conn.Close(websocket.StatusInternalError, "closing")
		}()

		reg := map[string]string{"id": cfg.ClientID}
		b, _ := json.Marshal(reg)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			return false, err
		}

		_, msg, err := conn.Read(ctx)
		if err != nil {
			return false, err
		}

		var ack struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(msg, &ack)
		cfg.ClientID = ack.ID
		logx.Log.Info().Str("server", cfg.ServerURL).Str("client_id", cfg.ClientID).Msg("connected to server")

		runCtx, cancel := context.WithCancel(ctx)
		go monitorProvider(runCtx, cfg.ProviderURL, cfg.Reconnect)

		relay := NewRelayClient(conn, cfg.ProviderURL)
		err = relay.Run(runCtx)
		cancel()
		if err != nil {
			return true, err
		}
		return true, nil
	}
	return agent.RunWithReconnect(ctx, cfg.Reconnect, connect)
}

func probeProvider(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}

func monitorProvider(ctx context.Context, url string, shouldReconnect bool) {
	attempt := 0
	for {
		if err := probeProvider(ctx, url); err != nil {
			logx.Log.Warn().Err(err).Msg("mcp provider unavailable; not_ready")
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
