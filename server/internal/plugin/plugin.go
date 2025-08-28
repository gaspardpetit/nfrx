package plugin

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

    "github.com/gaspardpetit/nfrx/sdk/spi"
)

// Plugin is implemented by all plugins.
type Plugin = spi.Plugin

// WorkerProvider is implemented by plugins that handle load-balanced workers.
type WorkerProvider = spi.WorkerProvider

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
}

// RegisterSurface mounts a plugin under /api/{id} and wires optional capabilities.
func RegisterSurface(parent chi.Router, p Plugin, preg *prometheus.Registry, state spi.StateRegistry) SurfaceMount {
	path := "/api/" + p.ID()
	sub := chi.NewRouter()
	parent.Mount(path, sub)

	sr := chiRouter{sub}
	mr := promRegistry{preg}
	p.RegisterRoutes(sr)
	p.RegisterMetrics(mr)
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
		reg.plugins = append(reg.plugins, p)
	}
	return reg
}

// Plugins returns all loaded plugins.
func (r *Registry) Plugins() []Plugin { return r.plugins }

// WorkerProviders returns plugins that implement WorkerProvider.
func (r *Registry) WorkerProviders() []WorkerProvider { return r.workers }

type chiRouter struct{ chi.Router }

func (r chiRouter) Handle(pattern string, h http.Handler) { r.Router.Handle(pattern, h) }
func (r chiRouter) Group(fn func(spi.Router)) {
	r.Router.Group(func(c chi.Router) { fn(chiRouter{c}) })
}
func (r chiRouter) Route(pattern string, fn func(spi.Router)) {
	r.Router.Route(pattern, func(c chi.Router) { fn(chiRouter{c}) })
}
func (r chiRouter) Use(mw ...spi.Middleware) {
	cms := make([]func(http.Handler) http.Handler, 0, len(mw))
	for _, m := range mw {
		cms = append(cms, m)
	}
	r.Router.Use(cms...)
}
func (r chiRouter) Get(pattern string, h http.Handler)  { r.Method("GET", pattern, h) }
func (r chiRouter) Post(pattern string, h http.Handler) { r.Method("POST", pattern, h) }

type promRegistry struct{ *prometheus.Registry }

func (r promRegistry) MustRegister(cs ...spi.Collector) {
	collectors := make([]prometheus.Collector, 0, len(cs))
	for _, c := range cs {
		collectors = append(collectors, c.(prometheus.Collector))
	}
	r.Registry.MustRegister(collectors...)
}
