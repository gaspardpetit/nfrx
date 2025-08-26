package extension

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	ctrlsrv "github.com/gaspardpetit/nfrx-server/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx-server/internal/serverstate"
)

// Plugin is implemented by all extensions.
type Plugin interface {
	ID() string
	RegisterRoutes(r chi.Router)
	RegisterMetrics(reg *prometheus.Registry)
	RegisterState(reg *serverstate.Registry)
}

// WorkerProvider is implemented by extensions that handle load-balanced workers.
type WorkerProvider interface {
	RegisterWebSocket(r chi.Router)
	Scheduler() ctrlsrv.Scheduler
}

// RelayProvider is implemented by extensions that manage client relays.
type RelayProvider interface {
	RegisterRelayEndpoints(r chi.Router)
}

// Context groups common facilities passed to extensions.
type Context struct {
	Router  chi.Router
	Metrics *prometheus.Registry
	State   *serverstate.Registry
}

// Registry holds loaded extensions and their optional capabilities.
type Registry struct {
	plugins []Plugin
	workers []WorkerProvider
	relays  []RelayProvider
}

// Load initializes extensions and returns a Registry describing their capabilities.
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

// Plugins returns all loaded extensions.
func (r *Registry) Plugins() []Plugin { return r.plugins }

// WorkerProviders returns extensions that implement WorkerProvider.
func (r *Registry) WorkerProviders() []WorkerProvider { return r.workers }

// RelayProviders returns extensions that implement RelayProvider.
func (r *Registry) RelayProviders() []RelayProvider { return r.relays }
