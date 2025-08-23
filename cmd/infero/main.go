package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gaspardpetit/infero/internal/config"
	"github.com/gaspardpetit/infero/internal/ctrl"
	"github.com/gaspardpetit/infero/internal/logx"
	"github.com/gaspardpetit/infero/internal/mcp"
	"github.com/gaspardpetit/infero/internal/metrics"
	"github.com/gaspardpetit/infero/internal/server"
	"github.com/gaspardpetit/infero/internal/serverstate"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func binaryName() string {
	b := filepath.Base(os.Args[0])
	if strings.HasPrefix(b, "infero-") {
		return strings.TrimPrefix(b, "infero-")
	}
	return b
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	var cfg config.ServerConfig
	cfg.BindFlags()
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "infero-%s version=%s sha=%s date=%s\n\n", binaryName(), version, buildSHA, buildDate)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Printf("infero-%s version=%s sha=%s date=%s\n", binaryName(), version, buildSHA, buildDate)
		return
	}

	if cfg.ConfigFile != "" {
		if err := cfg.LoadFile(cfg.ConfigFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			logx.Log.Fatal().Err(err).Str("path", cfg.ConfigFile).Msg("load config")
		}
	}

	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry(version, buildSHA, buildDate)
	metrics.Register(prometheus.DefaultRegisterer)
	metrics.SetServerBuildInfo(version, buildSHA, buildDate)
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	mcpReg := mcp.NewRegistry(cfg.RequestTimeout)
	handler := server.New(reg, metricsReg, sched, mcpReg, cfg)
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: handler}
	var metricsSrv *http.Server
	if cfg.MetricsAddr != fmt.Sprintf(":%d", cfg.Port) {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		metricsSrv = &http.Server{Addr: cfg.MetricsAddr, Handler: mux}
	}

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range sigCh {
			if serverstate.IsDraining() || cfg.DrainTimeout == 0 {
				logx.Log.Warn().Msg("termination requested")
				cancel()
				return
			}
			serverstate.StartDrain()
			if cfg.DrainTimeout > 0 {
				logx.Log.Info().Dur("timeout", cfg.DrainTimeout).Msg("draining; send SIGTERM again to terminate immediately")
				go func(d time.Duration) {
					time.Sleep(d)
					if serverstate.IsDraining() {
						logx.Log.Warn().Msg("drain timeout exceeded; terminating")
						cancel()
					}
				}(cfg.DrainTimeout)
			} else {
				logx.Log.Info().Msg("draining; send SIGTERM again to terminate immediately")
			}
		}
	}()
	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			logx.Log.Error().Err(err).Msg("server shutdown")
		}
	}()
	if metricsSrv != nil {
		go func() {
			<-ctx.Done()
			if err := metricsSrv.Shutdown(context.Background()); err != nil {
				logx.Log.Error().Err(err).Msg("metrics server shutdown")
			}
		}()
	}

	if cfg.APIKey != "" {
		logx.Log.Info().Msg("API key auth enabled")
	}
	if cfg.ClientKey != "" {
		logx.Log.Info().Msg("Client key required")
	}
	logx.Log.Info().Int("port", cfg.Port).Msg("server starting")
	if metricsSrv != nil {
		go func() {
			logx.Log.Info().Str("addr", cfg.MetricsAddr).Msg("metrics server starting")
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logx.Log.Error().Err(err).Msg("metrics server error")
			}
		}()
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logx.Log.Fatal().Err(err).Msg("server error")
	}
}
