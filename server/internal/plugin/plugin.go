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

// SurfaceMount represents a mounted plugin surface.
type SurfaceMount struct {
	Path    string
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

// RegisterSurface mounts a plugin under /api/{id} and wires optional capabilities.
func RegisterSurface(parent chi.Router, p Plugin, preg *prometheus.Registry, state spi.StateRegistry) SurfaceMount {
	path := "/api/" + p.ID()
	sub := chi.NewRouter()
	parent.Mount(path, sub)

	p.RegisterRoutes(sub)
	if wp, ok := p.(WorkerProvider); ok {
		wp.RegisterWebSocket(sub)
	}
	if rp, ok := p.(RelayProvider); ok {
		rp.RegisterRelayEndpoints(sub)
	}
	p.RegisterMetrics(preg)
	p.RegisterState(state)

	return SurfaceMount{Path: path, Router: sub, Metrics: preg, State: state}
}

// Load initializes plugins and returns a Registry describing their capabilities.
func Load(parent chi.Router, preg *prometheus.Registry, state spi.StateRegistry, plugins []Plugin) *Registry {
	reg := &Registry{}
	for _, p := range plugins {
		RegisterSurface(parent, p, preg, state)
		if wp, ok := p.(WorkerProvider); ok {
			reg.workers = append(reg.workers, wp)
		}
		if rp, ok := p.(RelayProvider); ok {
			reg.relays = append(reg.relays, rp)
		}
		reg.plugins = append(reg.plugins, p)
	}
	return reg
}

// Plugins returns all loaded plugins.
func (r *Registry) Plugins() []Plugin { return r.plugins }

// WorkerProviders returns plugins that implement WorkerProvider.
func (r *Registry) WorkerProviders() []WorkerProvider { return r.workers }

// RelayProviders returns plugins that implement RelayProvider.
func (r *Registry) RelayProviders() []RelayProvider { return r.relays }
