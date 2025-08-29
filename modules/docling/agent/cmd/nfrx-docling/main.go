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
    "github.com/gaspardpetit/nfrx/modules/docling/agent/internal/worker"
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
    if *showVersion { fmt.Printf("nfrx-docling version=%s sha=%s date=%s\n", version, buildSHA, buildDate); return }
    if cfg.ConfigFile != "" {
        if err := cfg.LoadFile(cfg.ConfigFile); err != nil && !errors.Is(err, os.ErrNotExist) { logx.Log.Fatal().Err(err).Str("path", cfg.ConfigFile).Msg("load config") }
    }
    logx.Configure(cfg.LogLevel)
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()
    if err := worker.Run(ctx, cfg); err != nil { logx.Log.Fatal().Err(err).Msg("agent exited") }
}

