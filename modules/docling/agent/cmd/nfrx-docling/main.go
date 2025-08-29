package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gaspardpetit/nfrx/core/logx"
	aconfig "github.com/gaspardpetit/nfrx/modules/docling/agent/internal/config"
	wp "github.com/gaspardpetit/nfrx/sdk/base/agent/workerproxy"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	var cfg aconfig.WorkerConfig
	cfg.BindFlags()
	flag.Parse()
	if *showVersion {
		fmt.Printf("nfrx-docling version=%s sha=%s date=%s\n", version, buildSHA, buildDate)
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
	// Bridge docling config to the generic worker-proxy runner.
	gcfg := wp.Config{
		ServerURL:      cfg.ServerURL,
		ClientKey:      cfg.ClientKey,
		BaseURL:        cfg.BaseURL,
		APIKey:         cfg.APIKey,
		ProbePath:      "/health",
		MaxConcurrency: cfg.MaxConcurrency,
		ClientID:       cfg.ClientID,
		ClientName:     cfg.ClientName,
		StatusAddr:     cfg.StatusAddr,
		MetricsAddr:    cfg.MetricsAddr,
		TokenBasename:  "docling",
		DrainTimeout:   cfg.DrainTimeout,
		RequestTimeout: cfg.RequestTimeout,
		Reconnect:      cfg.Reconnect,
		ConfigFile:     cfg.ConfigFile,
	}
	if err := wp.Run(ctx, gcfg); err != nil {
		logx.Log.Fatal().Err(err).Msg("agent exited")
	}
}
