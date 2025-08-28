package plugin

import (
    "net/http"

    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/sdk/spi"
)

// Factory is the common constructor for server plugins.
type Factory func(
    state spi.ServerState,
    connect http.Handler,
    workers spi.WorkerRegistry,
    sched spi.Scheduler,
    metrics spi.Metrics,
    stateProvider func() any,
    version, sha, date string,
    opts spi.Options,
    authMW spi.Middleware,
) spi.Plugin

var registry = map[string]Factory{}

// Register adds a factory for a given plugin ID.
func Register(id string, f Factory) { registry[id] = f }

// Get returns a factory by ID.
func Get(id string) (Factory, bool) { f, ok := registry[id]; return f, ok }

// IDs returns the list of registered plugin IDs.
func IDs() []string {
    out := make([]string, 0, len(registry))
    for k := range registry {
        out = append(out, k)
    }
    return out
}

// Wire built-in plugins.
func init() {
    Register("llm", func(state spi.ServerState, connect http.Handler, workers spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, stateProvider func() any, version, sha, date string, opts spi.Options, authMW spi.Middleware) spi.Plugin {
        return llm.New(state, connect, workers, sched, metrics, stateProvider, version, sha, date, opts, authMW)
    })
    Register("mcp", func(state spi.ServerState, connect http.Handler, workers spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, stateProvider func() any, version, sha, date string, opts spi.Options, authMW spi.Middleware) spi.Plugin {
        return mcp.New(state, connect, workers, sched, metrics, stateProvider, version, sha, date, opts, authMW)
    })
}
