package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gaspardpetit/nfrx/core/logx"
	aconfig "github.com/gaspardpetit/nfrx/modules/asr/agent/internal/config"
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
		fmt.Printf("nfrx-asr version=%s sha=%s date=%s\n", version, buildSHA, buildDate)
		return
	}
	if cfg.ConfigFile != "" {
		if err := cfg.LoadFile(cfg.ConfigFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			logx.Log.Fatal().Err(err).Str("path", cfg.ConfigFile).Msg("load config")
		}
	}
	logx.Configure(cfg.LogLevel)
	wp.SetBuildInfo(version, buildSHA, buildDate)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	probe := func(pctx context.Context) (wp.ProbeResult, error) {
		req, err := http.NewRequestWithContext(pctx, http.MethodGet, cfg.BaseURL+"/models", nil)
		if err != nil {
			return wp.ProbeResult{Ready: false}, err
		}
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return wp.ProbeResult{Ready: false}, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 400 {
			return wp.ProbeResult{Ready: false}, fmt.Errorf("status %s", resp.Status)
		}
		var v struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			return wp.ProbeResult{Ready: false}, err
		}
		models := make([]string, 0, len(v.Data))
		for _, m := range v.Data {
			models = append(models, m.ID)
		}
		return wp.ProbeResult{Ready: true, Models: models, MaxConcurrency: cfg.MaxConcurrency}, nil
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
		TokenBasename:  "asr",
		DrainTimeout:   cfg.DrainTimeout,
		RequestTimeout: cfg.RequestTimeout,
		Reconnect:      cfg.Reconnect,
		ConfigFile:     cfg.ConfigFile,
	}
	if err := wp.Run(ctx, gcfg); err != nil {
		logx.Log.Fatal().Err(err).Msg("agent exited")
	}
}
