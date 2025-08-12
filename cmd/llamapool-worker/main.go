package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/logx"
	"github.com/you/llamapool/internal/worker"
)

func main() {
	var cfg config.WorkerConfig
	cfg.BindFlags()
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log := logx.Log.Info().Str("worker_name", cfg.WorkerName)
	if cfg.WorkerKey != "" {
		log = log.Bool("auth", true)
	}
	log.Msg("worker starting")

	if err := worker.Run(ctx, cfg); err != nil {
		logx.Log.Fatal().Err(err).Msg("worker exited")
	}
}
