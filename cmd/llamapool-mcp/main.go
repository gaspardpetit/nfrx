package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/coder/websocket"
	"github.com/gaspardpetit/llamapool/internal/logx"
	"github.com/gaspardpetit/llamapool/internal/mcp"
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

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("llamapool-mcp version=%s sha=%s date=%s\n", version, buildSHA, buildDate)
		return
	}
	wsURL := getEnv("BROKER_WS_URL", "ws://localhost:8081/ws/relay")
	clientID := getEnv("CLIENT_ID", "")
	providerURL := getEnv("PROVIDER_URL", "http://127.0.0.1:7777/")
	token := getEnv("RELAY_AUTH_TOKEN", "")
	if clientID == "" {
		logx.Log.Fatal().Msg("CLIENT_ID required")
	}
	header := http.Header{}
	header.Set("X-Client-Id", clientID)
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	ctx := context.Background()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		logx.Log.Fatal().Err(err).Msg("dial broker")
	}
	relay := mcp.NewRelayClient(conn, providerURL)
	if err := relay.Run(ctx); err != nil {
		logx.Log.Error().Err(err).Msg("relay stopped")
	}
}
