package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/logx"
	"github.com/you/llamapool/internal/metrics"
	"github.com/you/llamapool/internal/server"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func main() {
	var cfg config.ServerConfig
	cfg.BindFlags()
	flag.Parse()

	reg := ctrl.NewRegistry()
	metricsReg := ctrl.NewMetricsRegistry(version, buildSHA, buildDate)
	metrics.Register(prometheus.DefaultRegisterer)
	metrics.SetServerBuildInfo(version, buildSHA, buildDate)
	sched := &ctrl.LeastBusyScheduler{Reg: reg}
	handler := server.New(reg, metricsReg, sched, cfg)
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: handler}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	if cfg.APIKey != "" {
		logx.Log.Info().Msg("API key auth enabled")
	}
	if cfg.WorkerKey != "" {
		logx.Log.Info().Msg("Worker key required")
	}
	logx.Log.Info().Int("port", cfg.Port).Msg("server starting")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logx.Log.Fatal().Err(err).Msg("server error")
	}
}
