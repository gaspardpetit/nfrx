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
	aconfig "github.com/gaspardpetit/nfrx/modules/mcp/agent/internal/config"
	"github.com/gaspardpetit/nfrx/modules/mcp/agent/internal/mcp"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	var cfg aconfig.MCPConfig
	cfg.BindFlags()
	flag.Parse()

	overrides := map[string]string{}
	flag.CommandLine.Visit(func(f *flag.Flag) {
		overrides[f.Name] = f.Value.String()
	})
	if *showVersion {
		fmt.Printf("nfrx-mcp version=%s sha=%s date=%s\n", version, buildSHA, buildDate)
		return
	}
	if cfg.ConfigFile != "" {
		if err := cfg.LoadFile(cfg.ConfigFile); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				logx.Log.Fatal().Err(err).Str("path", cfg.ConfigFile).Msg("load config")
			}
		} else {
			for name, value := range overrides {
				if f := flag.CommandLine.Lookup(name); f != nil {
					if err := f.Value.Set(value); err != nil {
						logx.Log.Fatal().Err(err).Str("flag", name).Msg("restore cli flag")
					}
				}
			}
		}
	}
	logx.Configure(cfg.LogLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := mcp.Run(ctx, cfg); err != nil {
		logx.Log.Fatal().Err(err).Msg("relay stopped")
	}
}
