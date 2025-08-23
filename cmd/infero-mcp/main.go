package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/gaspardpetit/infero/internal/config"
	"github.com/gaspardpetit/infero/internal/logx"
	"github.com/gaspardpetit/infero/internal/mcp"
	reconnect "github.com/gaspardpetit/infero/internal/reconnect"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func probeProvider(ctx context.Context, url string) error {
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
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("status %s", resp.Status)
	}
	return nil
}

func monitorProvider(ctx context.Context, url string, shouldReconnect bool) {
	attempt := 0
	for {
		err := probeProvider(ctx, url)
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

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	var cfg config.MCPConfig
	cfg.BindFlags()
	flag.Parse()
	if *showVersion {
		fmt.Printf("infero-mcp version=%s sha=%s date=%s\n", version, buildSHA, buildDate)
		return
	}
	if cfg.ConfigFile != "" {
		if err := cfg.LoadFile(cfg.ConfigFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			logx.Log.Fatal().Err(err).Str("path", cfg.ConfigFile).Msg("load config")
		}
	}

	header := http.Header{}
	reconnectFlag := cfg.Reconnect
	metricsAddr := cfg.MetricsAddr
	requestTimeout := cfg.RequestTimeout
	wsURL := cfg.ServerURL
	clientKey := cfg.ClientKey
	clientID := cfg.ClientID
	clientName := cfg.ClientName
	providerURL := cfg.ProviderURL
	authToken := cfg.AuthToken

	if metricsAddr != "" {
		if _, err := mcp.StartMetricsServer(context.Background(), metricsAddr); err != nil {
			logx.Log.Fatal().Err(err).Msg("metrics server")
		}
		logx.Log.Info().Str("addr", metricsAddr).Msg("metrics server started")
	}

	attempt := 0
	for {
		ctx := context.Background()
		conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
		if err != nil {
			if !reconnectFlag {
				logx.Log.Fatal().Err(err).Msg("dial broker")
			}
			delay := reconnect.Delay(attempt)
			attempt++
			logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("dial broker failed; retrying")
			time.Sleep(delay)
			continue
		}
		reg := map[string]string{"id": clientID, "client_name": clientName}
		if clientKey != "" {
			reg["client_key"] = clientKey
		}
		b, _ := json.Marshal(reg)
		if err := conn.Write(ctx, websocket.MessageText, b); err != nil {
			_ = conn.Close(websocket.StatusInternalError, "closing")
			logx.Log.Error().Err(err).Msg("register")
			return
		}
		_, msg, err := conn.Read(ctx)
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "closing")
			logx.Log.Error().Err(err).Msg("register ack")
			return
		}
		var ack struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(msg, &ack)
		clientID = ack.ID
		logx.Log.Info().Str("server", wsURL).Str("client_id", clientID).Str("client_name", clientName).Msg("connected to server")
		attempt = 0

		runCtx, cancel := context.WithCancel(ctx)
		go monitorProvider(runCtx, providerURL, reconnectFlag)

		relay := mcp.NewRelayClient(conn, providerURL, authToken, requestTimeout)
		if err := relay.Run(runCtx); err != nil {
			cancel()
			_ = conn.Close(websocket.StatusInternalError, "closing")
			if !reconnectFlag {
				logx.Log.Error().Err(err).Msg("relay stopped")
				return
			}
			delay := reconnect.Delay(attempt)
			attempt++
			logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("relay stopped; reconnecting")
			time.Sleep(delay)
			continue
		}
		cancel()
		_ = conn.Close(websocket.StatusNormalClosure, "closing")
		return
	}
}
