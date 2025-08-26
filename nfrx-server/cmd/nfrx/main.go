package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"

	"github.com/gaspardpetit/nfrx-sdk/config"
	ctrlpb "github.com/gaspardpetit/nfrx-sdk/ctrl"
	"github.com/gaspardpetit/nfrx-sdk/logx"
	"github.com/gaspardpetit/nfrx-server/internal/controlgrpc"
	"github.com/gaspardpetit/nfrx-server/internal/extension"
	llmserver "github.com/gaspardpetit/nfrx-server/internal/llmserver"
	mcphub "github.com/gaspardpetit/nfrx-server/internal/mcphub"
	mcpserver "github.com/gaspardpetit/nfrx-server/internal/mcpserver"
	"github.com/gaspardpetit/nfrx-server/internal/server"
	"github.com/gaspardpetit/nfrx-server/internal/serverstate"
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
	var cfg config.ServerConfig
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

	if cfg.RedisAddr != "" {
		rs, err := serverstate.NewRedisStore(cfg.RedisAddr)
		if err != nil {
			logx.Log.Fatal().Err(err).Msg("connect redis")
		}
		serverstate.UseStore(rs)
		logx.Log.Info().Str("addr", cfg.RedisAddr).Msg("using redis state store")
	}

	stateReg := serverstate.NewRegistry()
	var plugins []extension.Plugin
	var mcpReg *mcpserver.Plugin
	if hasPlugin(cfg.Plugins, "mcp") {
		mcpReg = mcpserver.New(cfg, cfg.PluginOptions["mcp"])
		plugins = append(plugins, mcpReg)
	}
	var mcpHub *mcphub.Registry
	if mcpReg != nil {
		mcpHub = mcpReg.Registry()
	}
	var llm *llmserver.Plugin
	if hasPlugin(cfg.Plugins, "llm") {
		llm = llmserver.New(cfg, version, buildSHA, buildDate, mcpHub, cfg.PluginOptions["llm"])
		plugins = append(plugins, llm)
	}
	handler := server.New(cfg, stateReg, plugins)
	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Port), Handler: handler}
	var metricsSrv *http.Server
	if cfg.MetricsAddr != fmt.Sprintf(":%d", cfg.Port) {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		metricsSrv = &http.Server{Addr: cfg.MetricsAddr, Handler: mux}
	}
	var grpcSrv *grpc.Server
	if llm != nil {
		grpcSrv = grpc.NewServer()
		ctrlpb.RegisterControlServer(grpcSrv, controlgrpc.New(llm.Registry(), llm.MetricsRegistry(), cfg.ClientKey))
		go func() {
			addr := fmt.Sprintf(":%d", cfg.Port+1)
			lis, err := net.Listen("tcp", addr)
			if err != nil {
				logx.Log.Fatal().Err(err).Msg("control grpc listen")
			}
			logx.Log.Info().Str("addr", addr).Msg("control grpc starting")
			if err := grpcSrv.Serve(lis); err != nil {
				logx.Log.Error().Err(err).Msg("control grpc error")
			}
		}()
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
		if grpcSrv != nil {
			grpcSrv.GracefulStop()
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
