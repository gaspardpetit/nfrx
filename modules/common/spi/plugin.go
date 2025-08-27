package spi

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

type Plugin interface {
	ID() string
	RegisterRoutes(r chi.Router)
	RegisterMetrics(reg *prometheus.Registry)
	RegisterState(reg StateRegistry)
}

type WorkerProvider interface {
	RegisterWebSocket(r chi.Router)
	Scheduler() Scheduler
}

type RelayProvider interface {
	RegisterRelayEndpoints(r chi.Router)
}
