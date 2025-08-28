package spi

import "net/http"

// Middleware represents an HTTP middleware function.
type Middleware func(http.Handler) http.Handler

// Router abstracts the HTTP router used by plugins.
type Router interface {
	Handle(pattern string, h http.Handler)
	Group(fn func(r Router))
	Route(pattern string, fn func(r Router))
	Use(mw ...Middleware)
	Get(pattern string, h http.Handler)
	Post(pattern string, h http.Handler)
}

// Collector represents a metric collector.
type Collector interface{}

// MetricsRegistry abstracts the Prometheus registry used by plugins.
type MetricsRegistry interface {
	MustRegister(...Collector)
}

// Plugin is implemented by all plugins.
type Plugin interface {
	ID() string
	RegisterRoutes(r Router)
	RegisterMetrics(reg MetricsRegistry)
	RegisterState(reg StateRegistry)
}

// WorkerProvider is implemented by plugins that handle load-balanced workers.
type WorkerProvider interface {
	Scheduler() Scheduler
}
