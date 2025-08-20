package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/logx"
	"github.com/gaspardpetit/llamapool/internal/mcp"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func binaryName() string {
	b := filepath.Base(os.Args[0])
	if strings.HasPrefix(b, "llamapool-") {
		return strings.TrimPrefix(b, "llamapool-")
	}
	return b
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	var cfg config.MCPRelayConfig
	cfg.BindFlags()
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "llamapool-%s version=%s sha=%s date=%s\n\n", binaryName(), version, buildSHA, buildDate)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Printf("llamapool-%s version=%s sha=%s date=%s\n", binaryName(), version, buildSHA, buildDate)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range sigCh {
			logx.Log.Warn().Msg("termination requested")
			cancel()
			return
		}
	}()

	log := logx.Log.Info().Str("client_id", cfg.ClientID)
	if cfg.Reconnect {
		log = log.Bool("reconnect", true)
	}
	log.Msg("mcp starting")

	if err := mcp.Run(ctx, cfg); err != nil {
		logx.Log.Fatal().Err(err).Msg("mcp exited")
	}
}
