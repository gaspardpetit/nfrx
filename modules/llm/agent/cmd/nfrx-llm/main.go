package main

import (
    "context"
    "errors"
    "flag"
    "fmt"
    "os"
    "os/signal"
    "path/filepath"
    "strings"
    "syscall"
    "time"

    "github.com/gaspardpetit/nfrx/core/logx"
    aconfig "github.com/gaspardpetit/nfrx/modules/llm/agent/internal/config"
    "github.com/gaspardpetit/nfrx/modules/llm/agent/internal/ollama"
    wp "github.com/gaspardpetit/nfrx/sdk/base/agent/workerproxy"
    "strconv"
)

var (
	version   = "dev"
	buildSHA  = "unknown"
	buildDate = "unknown"
)

func binaryName() string {
	b := filepath.Base(os.Args[0])
	if strings.HasPrefix(b, "nfrx-") {
		return strings.TrimPrefix(b, "nfrx-")
	}
	return b
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	var cfg aconfig.WorkerConfig
	cfg.BindFlags()
	flag.Usage = func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "nfrx-%s version=%s sha=%s date=%s\n\n", binaryName(), version, buildSHA, buildDate)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Printf("nfrx-%s version=%s sha=%s date=%s\n", binaryName(), version, buildSHA, buildDate)
		return
	}

	if cfg.ConfigFile != "" {
		if err := cfg.LoadFile(cfg.ConfigFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			logx.Log.Fatal().Err(err).Str("path", cfg.ConfigFile).Msg("load config")
		}
	}
	logx.Configure(cfg.LogLevel)

    wp.SetBuildInfo(version, buildSHA, buildDate)

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
            for range sigCh {
                if wp.IsDraining() || cfg.DrainTimeout == 0 {
                    logx.Log.Warn().Msg("termination requested")
                    wp.SetState("terminating")
                    cancel()
                    return
                }
                wp.StartDrain()
                if cfg.DrainTimeout > 0 {
                    logx.Log.Info().Dur("timeout", cfg.DrainTimeout).Msg("draining; send SIGTERM again to terminate immediately")
                    go func(d time.Duration) {
                        time.Sleep(d)
                        if wp.IsDraining() {
                            logx.Log.Warn().Msg("drain timeout exceeded; terminating")
                            wp.SetState("terminating")
                            cancel()
                        }
                    }(cfg.DrainTimeout)
                } else {
                    logx.Log.Info().Msg("draining; send SIGTERM again to terminate immediately")
                }
            }
        }()

    log := logx.Log.Info().Str("client_name", cfg.ClientName)
    if cfg.ClientKey != "" {
        log = log.Bool("auth", true)
    }
    log.Msg("worker starting")
    // Bridge LLM config to the generic worker-proxy runner with a custom probe
    // that discovers models via Ollama tags (CompletionBaseURL typically ends with /v1).
    client := ollama.New(strings.TrimSuffix(cfg.CompletionBaseURL, "/v1"))
    probe := func(pctx context.Context) (wp.ProbeResult, error) {
        models, err := client.Health(pctx)
        if err != nil { return wp.ProbeResult{Ready: false}, err }
        return wp.ProbeResult{Ready: true, Models: models, MaxConcurrency: cfg.MaxConcurrency}, nil
    }
    agentCfg := map[string]string{}
    if cfg.EmbeddingBatchSize > 0 { agentCfg["embedding_batch_size"] = strconv.Itoa(cfg.EmbeddingBatchSize) }
    gcfg := wp.Config{
        ServerURL:          cfg.ServerURL,
        ClientKey:          cfg.ClientKey,
        BaseURL:            cfg.CompletionBaseURL,
        APIKey:             cfg.CompletionAPIKey,
        ProbeFunc:          probe,
        ProbeInterval:      cfg.ModelPollInterval,
        MaxConcurrency:     cfg.MaxConcurrency,
        ClientID:           cfg.ClientID,
        ClientName:         cfg.ClientName,
        StatusAddr:         cfg.StatusAddr,
        MetricsAddr:        cfg.MetricsAddr,
        TokenBasename:      "llm",
        DrainTimeout:       cfg.DrainTimeout,
        RequestTimeout:     cfg.RequestTimeout,
        Reconnect:          cfg.Reconnect,
        ConfigFile:         cfg.ConfigFile,
        AgentConfig:        agentCfg,
    }
    if err := wp.Run(ctx, gcfg); err != nil {
        logx.Log.Fatal().Err(err).Msg("worker exited")
    }
}
