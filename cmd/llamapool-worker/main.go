package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/logx"
	"github.com/gaspardpetit/llamapool/internal/worker"
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
	var cfg config.WorkerConfig
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

	worker.SetBuildInfo(version, buildSHA, buildDate)

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		for range sigCh {
			if worker.IsDraining() || cfg.DrainTimeout == 0 {
				logx.Log.Warn().Msg("termination requested")
				worker.SetState("terminating")
				cancel()
				return
			}
			worker.StartDrain()
			if cfg.DrainTimeout > 0 {
				logx.Log.Info().Dur("timeout", cfg.DrainTimeout).Msg("draining; send SIGTERM again to terminate immediately")
				go func(d time.Duration) {
					time.Sleep(d)
					if worker.IsDraining() {
						logx.Log.Warn().Msg("drain timeout exceeded; terminating")
						worker.SetState("terminating")
						cancel()
					}
				}(cfg.DrainTimeout)
			} else {
				logx.Log.Info().Msg("draining; send SIGTERM again to terminate immediately")
			}
		}
	}()

	log := logx.Log.Info().Str("worker_name", cfg.WorkerName)
	if cfg.WorkerKey != "" {
		log = log.Bool("auth", true)
	}
	log.Msg("worker starting")

	if err := worker.Run(ctx, cfg); err != nil {
		logx.Log.Fatal().Err(err).Msg("worker exited")
	}
}
