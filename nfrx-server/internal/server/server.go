package server

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gaspardpetit/nfrx-sdk/config"
	"github.com/gaspardpetit/nfrx-server/internal/api"
	"github.com/gaspardpetit/nfrx-server/internal/extension"
	"github.com/gaspardpetit/nfrx-server/internal/serverstate"
)

// New constructs the HTTP handler for the server.
func New(cfg config.ServerConfig, stateReg *serverstate.Registry, plugins []extension.Plugin) http.Handler {
	r := chi.NewRouter()
	if len(cfg.AllowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins: cfg.AllowedOrigins,
			AllowedMethods: []string{"GET", "POST", "OPTIONS"},
			AllowedHeaders: []string{"*"},
		}))
	}
	for _, m := range api.MiddlewareChain() {
		r.Use(m)
	}

	if stateReg == nil {
		stateReg = serverstate.NewRegistry()
	}
	preg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = preg
	prometheus.DefaultGatherer = preg
	pregistry := extension.Load(extension.Context{Router: r, Metrics: preg, State: stateReg}, plugins)
	for _, wp := range pregistry.WorkerProviders() {
		wp.RegisterWebSocket(r)
	}
	for _, rp := range pregistry.RelayProviders() {
		rp.RegisterRelayEndpoints(r)
	}

	r.Get("/state", StateHandler())

	if cfg.MetricsAddr == fmt.Sprintf(":%d", cfg.Port) {
		r.Handle("/metrics", promhttp.HandlerFor(preg, promhttp.HandlerOpts{}))
	}

	return r
}
