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
    "sort"
    "strings"
    "syscall"
    "time"

    "github.com/prometheus/client_golang/prometheus/promhttp"

    "github.com/gaspardpetit/nfrx/core/logx"
    "github.com/gaspardpetit/nfrx/server/internal/adapters"
    "github.com/gaspardpetit/nfrx/server/internal/api"
    "github.com/gaspardpetit/nfrx/server/internal/config"
    ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
    "github.com/gaspardpetit/nfrx/server/internal/metrics"
    "github.com/gaspardpetit/nfrx/server/internal/plugin"
    "github.com/gaspardpetit/nfrx/server/internal/server"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
    spicontracts "github.com/gaspardpetit/nfrx/sdk/spi"
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

func hasPlugin(list []string, name string) bool {
	for _, p := range list {
		if p == name {
			return true
		}
	}
	return false
}

func main() {
    showVersion := flag.Bool("version", false, "print version and exit")
    showPluginHelp := flag.Bool("help-plugins", false, "print extension options and exit")
    var cfg config.ServerConfig
    // Resolve config with precedence: defaults < file < env < args
    cfg.SetDefaults()
    cfg.ApplyEnv() // allows CONFIG_FILE from env
    // Allow --config to override file path before loading it
    for i := 1; i < len(os.Args); i++ {
        a := os.Args[i]
        if a == "--config" && i+1 < len(os.Args) {
            cfg.ConfigFile = os.Args[i+1]
            break
        }
        if strings.HasPrefix(a, "--config=") {
            cfg.ConfigFile = strings.TrimPrefix(a, "--config=")
            break
        }
    }
    if cfg.ConfigFile != "" {
        if err := cfg.LoadFile(cfg.ConfigFile); err != nil && !errors.Is(err, os.ErrNotExist) {
            logx.Log.Fatal().Err(err).Str("path", cfg.ConfigFile).Msg("load config")
        }
    }
    // Overlay env (after file) and then bind flags; args parsed below
    cfg.ApplyEnv()
    // Overlay plugin options from environment using descriptors
    if cfg.PluginOptions == nil { cfg.PluginOptions = map[string]map[string]string{} }
    for id, d := range plugin.Descriptors() {
        for _, a := range d.Args {
            if a.Env == "" { continue }
            if v := os.Getenv(a.Env); v != "" {
                po := cfg.PluginOptions[id]
                if po == nil { po = map[string]string{} }
                po[a.ID] = v
                cfg.PluginOptions[id] = po
            }
        }
    }
    // Bind core flags
    cfg.BindFlagsFromCurrent()
    // Dynamically bind extension flags using descriptors
    for id, d := range plugin.Descriptors() {
        for _, a := range d.Args {
            if a.Flag == "" { continue }
            name := strings.TrimPrefix(a.Flag, "--")
            // Capture id and arg id
            pid, aid := id, a.ID
            flag.Func(name, fmt.Sprintf("extension option (%s.%s)", id, a.ID), func(v string) error {
                cfg.SetPluginOption(pid, aid, v)
                return nil
            })
        }
    }
    flag.Usage = func() {
        _, _ = fmt.Fprintf(flag.CommandLine.Output(), "nfrx-%s version=%s sha=%s date=%s\n\n", binaryName(), version, buildSHA, buildDate)
        flag.PrintDefaults()
        // Print extension descriptors
        fmt.Println()
        fmt.Println("Extensions:")
        ids := plugin.IDs()
        sort.Strings(ids)
        for _, id := range ids {
            if d, ok := plugin.Descriptor(id); ok {
                fmt.Printf("  - %s (%s)\n", d.Name, d.ID)
                if d.Summary != "" {
                    fmt.Printf("    %s\n", d.Summary)
                }
                for _, a := range d.Args {
                    fmt.Printf("    * %s: %s\n", a.ID, a.Description)
                    if a.Flag != "" {
                        fmt.Printf("      flag: %s\n", a.Flag)
                    }
                    if a.Env != "" {
                        fmt.Printf("      env: %s\n", a.Env)
                    }
                    if a.YAML != "" {
                        fmt.Printf("      yaml: %s\n", a.YAML)
                    }
                    if a.Type != "" || a.Default != "" || a.Example != "" {
                        fmt.Printf("      type: %s  default: %s", a.Type, a.Default)
                        if a.Example != "" {
                            fmt.Printf("  example: %s", a.Example)
                        }
                        fmt.Println()
                    }
                    if a.Deprecated {
                        repl := a.Replacement
                        if repl == "" { repl = "(none)" }
                        fmt.Printf("      deprecated; replacement: %s\n", repl)
                    }
                }
            }
        }
    }
    flag.Parse()
    if *showVersion {
        fmt.Printf("nfrx-%s version=%s sha=%s date=%s\n", binaryName(), version, buildSHA, buildDate)
        return
    }
    if *showPluginHelp {
        // Trigger usage to print plugin help then exit
        flag.Usage()
        return
    }

    // cfg now reflects defaults <- file <- env <- args
    logx.Configure(cfg.LogLevel)
    // Set build info metric (collectors are registered in server.New)
    metrics.SetServerBuildInfo(version, buildSHA, buildDate)

	if cfg.RedisAddr != "" {
		rs, err := serverstate.NewRedisStore(cfg.RedisAddr)
		if err != nil {
			logx.Log.Fatal().Err(err).Msg("connect redis")
		}
		serverstate.UseStore(rs)
		logx.Log.Info().Str("addr", cfg.RedisAddr).Msg("using redis state store")
	}

    stateReg := serverstate.NewRegistry()
    var plugins []plugin.Plugin

    // Build common server options for all extensions
    commonOpts := spicontracts.Options{
        RequestTimeout:        cfg.RequestTimeout,
        ClientKey:             cfg.ClientKey,
        PluginOptions:         cfg.PluginOptions,
    }

    // Build common SPI dependencies (worker control plane) once; plugins may ignore what they don't use
    reg := ctrlsrv.NewRegistry()
    metricsReg := ctrlsrv.NewMetricsRegistry(version, buildSHA, buildDate)
    sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
    connect := ctrlsrv.WSHandler(reg, metricsReg, cfg.ClientKey)
    wr := adapters.NewWorkerRegistry(reg)
    sc := adapters.NewScheduler(sched)
    mx := adapters.NewMetrics(metricsReg)
    authMW := (func(http.Handler) http.Handler)(nil)
    if cfg.APIKey != "" {
        authMW = api.APIKeyMiddleware(cfg.APIKey)
    }
    stateProvider := func() any { return metricsReg.Snapshot() }

    ids := cfg.Plugins
    if len(ids) == 1 && ids[0] == "*" {
        ids = plugin.IDs()
        sort.Strings(ids)
    }
    // Apply descriptor defaults into plugin options when absent
    optsWithDefaults := commonOpts
    // Copy plugin options to avoid mutating cfg
    optsWithDefaults.PluginOptions = map[string]map[string]string{}
    for k, v := range commonOpts.PluginOptions {
        mv := map[string]string{}
        for kk, vv := range v { mv[kk] = vv }
        optsWithDefaults.PluginOptions[k] = mv
    }
    for _, id := range ids {
        if d, ok := plugin.Descriptor(id); ok {
            po := optsWithDefaults.PluginOptions[id]
            if po == nil { po = map[string]string{} }
            for _, a := range d.Args {
                if a.Default != "" {
                    if _, exists := po[a.ID]; !exists || po[a.ID] == "" {
                        po[a.ID] = a.Default
                    }
                }
            }
            optsWithDefaults.PluginOptions[id] = po
        }
    }
    hasWorker := false
    for _, id := range ids {
        if f, ok := plugin.Get(id); ok {
            p := f(adapters.ServerState{}, connect, wr, sc, mx, stateProvider, version, buildSHA, buildDate, optsWithDefaults, authMW)
            plugins = append(plugins, p)
            if _, ok := p.(plugin.WorkerProvider); ok {
                hasWorker = true
            }
        } else {
            logx.Log.Warn().Str("plugin", id).Msg("unknown plugin; skipping")
        }
    }
    if hasWorker {
        // Pruning loop for worker-style agents
        go func() {
            tick := commonOpts.AgentHeartbeatInterval
            if tick == 0 {
                tick = ctrlsrv.HeartbeatInterval
            }
            expire := commonOpts.AgentHeartbeatExpiry
            if expire == 0 {
                expire = ctrlsrv.HeartbeatExpiry
            }
            ticker := time.NewTicker(tick)
            for range ticker.C {
                reg.PruneExpired(expire)
            }
        }()
    }
	handler := server.New(cfg, stateReg, plugins)
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
