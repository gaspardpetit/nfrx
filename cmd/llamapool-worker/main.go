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

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/logx"
	"github.com/you/llamapool/internal/worker"
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
