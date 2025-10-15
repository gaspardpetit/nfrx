package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
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
	cliOverrides := captureCLIOverrides(os.Args[1:])
	flag.Parse()
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
			flag.CommandLine.Visit(func(f *flag.Flag) {
				if value, ok := cliOverrides[f.Name]; ok {
					if err := f.Value.Set(value); err != nil {
						logx.Log.Fatal().Err(err).Str("flag", f.Name).Msg("restore cli flag")
					}
				}
			})
		}
	}
	logx.Configure(cfg.LogLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := mcp.Run(ctx, cfg); err != nil {
		logx.Log.Fatal().Err(err).Msg("relay stopped")
	}
}

func captureCLIOverrides(args []string) map[string]string {
	overrides := map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if name == "" {
			continue
		}
		var value string
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			value = name[eq+1:]
			name = name[:eq]
		} else {
			if i+1 < len(args) && (len(args[i+1]) == 0 || args[i+1][0] != '-') {
				value = args[i+1]
				i++
			} else {
				value = "true"
			}
		}
		overrides[name] = value
	}
	return overrides
}
