package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gaspardpetit/nfrx/internal/config"
	"github.com/gaspardpetit/nfrx/internal/logx"
	"github.com/gaspardpetit/nfrx/modules/mcp/agent/internal/mcp"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	var cfg config.MCPConfig
	cfg.BindFlags()
	flag.Parse()
	if *showVersion {
		fmt.Printf("nfrx-mcp version=%s sha=%s date=%s\n", version, buildSHA, buildDate)
		return
	}
	if cfg.ConfigFile != "" {
		if err := cfg.LoadFile(cfg.ConfigFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			logx.Log.Fatal().Err(err).Str("path", cfg.ConfigFile).Msg("load config")
		}
	}
	logx.Configure(cfg.LogLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := mcp.Run(ctx, cfg); err != nil {
		logx.Log.Fatal().Err(err).Msg("relay stopped")
	}
}
