package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/gaspardpetit/llamapool/internal/logx"
	"github.com/gaspardpetit/llamapool/internal/mcp"
	reconnect "github.com/gaspardpetit/llamapool/internal/reconnect"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func getEnvBool(k string, d bool) bool {
	if v := os.Getenv(k); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return d
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
		err := probeProvider(ctx, url)
		if err != nil {
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

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	reconnectFlag := getEnvBool("RECONNECT", false)
	flag.BoolVar(&reconnectFlag, "reconnect", reconnectFlag, "reconnect to server on failure")
	flag.BoolVar(&reconnectFlag, "r", reconnectFlag, "short for --reconnect")
	flag.Parse()
	if *showVersion {
		fmt.Printf("llamapool-mcp version=%s sha=%s date=%s\n", version, buildSHA, buildDate)
		return
	}

	wsURL := getEnv("SERVER_URL", "ws://localhost:8080/api/mcp/connect")
	clientID := getEnv("CLIENT_ID", "")
	providerURL := getEnv("PROVIDER_URL", "http://127.0.0.1:7777/")
	header := http.Header{}

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
		reg := map[string]string{"id": clientID}
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
		logx.Log.Info().Str("server", wsURL).Str("client_id", clientID).Msg("connected to server")
		attempt = 0

		runCtx, cancel := context.WithCancel(ctx)
		go monitorProvider(runCtx, providerURL, reconnectFlag)

		relay := mcp.NewRelayClient(conn, providerURL)
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
