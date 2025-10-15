package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
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

	overrides := map[string]string{}
	flag.CommandLine.Visit(func(f *flag.Flag) {
		overrides[f.Name] = f.Value.String()
	})
	if *showVersion {
		fmt.Printf("nfrx-docling version=%s sha=%s date=%s\n", version, buildSHA, buildDate)
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
	// Bridge docling config to the generic worker-proxy runner.
	probe := func(pctx context.Context) (wp.ProbeResult, error) {
		req, err := http.NewRequestWithContext(pctx, http.MethodGet, cfg.BaseURL+"/health", nil)
		if err != nil {
			return wp.ProbeResult{Ready: false}, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return wp.ProbeResult{Ready: false}, err
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			return wp.ProbeResult{Ready: false}, fmt.Errorf("status %s", resp.Status)
		}
		return wp.ProbeResult{Ready: true, MaxConcurrency: cfg.MaxConcurrency}, nil
	}
	gcfg := wp.Config{
		ServerURL:      cfg.ServerURL,
		ClientKey:      cfg.ClientKey,
		BaseURL:        cfg.BaseURL,
		APIKey:         cfg.APIKey,
		ProbeFunc:      probe,
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
