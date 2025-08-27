package plugin

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gaspardpetit/nfrx/modules/common/spi"
)

// Plugin is implemented by all plugins.
type Plugin = spi.Plugin

// WorkerProvider is implemented by plugins that handle load-balanced workers.
type WorkerProvider = spi.WorkerProvider

// RelayProvider is implemented by plugins that manage client relays.
type RelayProvider = spi.RelayProvider

type RelayWS = spi.RelayWS

// Context groups common facilities passed to plugins.
type Context struct {
	Router  chi.Router
	Metrics *prometheus.Registry
	State   spi.StateRegistry
}

// Registry holds loaded plugins and their optional capabilities.
type Registry struct {
	plugins []Plugin
	workers []WorkerProvider
	relays  []RelayProvider
}

// Load initializes plugins and returns a Registry describing their capabilities.
func Load(ctx Context, plugins []Plugin) *Registry {
	reg := &Registry{}
	for _, p := range plugins {
		p.RegisterRoutes(ctx.Router)
		p.RegisterMetrics(ctx.Metrics)
		p.RegisterState(ctx.State)
		reg.plugins = append(reg.plugins, p)
		if wp, ok := p.(WorkerProvider); ok {
			reg.workers = append(reg.workers, wp)
		}
		if rp, ok := p.(RelayProvider); ok {
			reg.relays = append(reg.relays, rp)
		}
	}
	return reg
}

// Plugins returns all loaded plugins.
func (r *Registry) Plugins() []Plugin { return r.plugins }

// WorkerProviders returns plugins that implement WorkerProvider.
func (r *Registry) WorkerProviders() []WorkerProvider { return r.workers }

// RelayProviders returns plugins that implement RelayProvider.
func (r *Registry) RelayProviders() []RelayProvider { return r.relays }
