package plugin

import (
    "net/http"

    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/sdk/spi"
)

// Factory is the common constructor for server extensions.
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

var (
    registry    = map[string]Factory{}
    descriptors = map[string]spi.PluginDescriptor{}
)

// Register adds a factory and descriptor for a given plugin ID.
func Register(id string, f Factory, d spi.PluginDescriptor) {
    registry[id] = f
    if d.ID == "" {
        d.ID = id
    }
    descriptors[id] = d
}

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

// Descriptor returns the descriptor for a plugin ID.
func Descriptor(id string) (spi.PluginDescriptor, bool) { d, ok := descriptors[id]; return d, ok }

// Descriptors returns all known descriptors.
func Descriptors() map[string]spi.PluginDescriptor { return descriptors }

// Wire built-in plugins.
func init() {
    Register("llm", func(state spi.ServerState, connect http.Handler, workers spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, stateProvider func() any, version, sha, date string, opts spi.Options, authMW spi.Middleware) spi.Plugin {
        return llm.New(state, connect, workers, sched, metrics, stateProvider, version, sha, date, opts, authMW)
    }, spi.PluginDescriptor{
        ID:      "llm",
        Name:    "LLM Gateway",
        Summary: "OpenAI-compatible API over an agent pool",
        Args: []spi.ArgSpec{
            {
                ID:          "max_parallel_embeddings",
                Flag:        "--llm-max-parallel-embeddings",
                Env:         "LLM_MAX_PARALLEL_EMBEDDINGS",
                YAML:        "plugin_options.llm.max_parallel_embeddings",
                Type:        spi.ArgInt,
                Default:     "8",
                Example:     "16",
                Description: "Maximum agents to split embeddings across",
            },
        },
    })
    Register("mcp", func(state spi.ServerState, connect http.Handler, workers spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, stateProvider func() any, version, sha, date string, opts spi.Options, authMW spi.Middleware) spi.Plugin {
        return mcp.New(state, connect, workers, sched, metrics, stateProvider, version, sha, date, opts, authMW)
    }, spi.PluginDescriptor{
        ID:      "mcp",
        Name:    "MCP Relay",
        Summary: "Relay for Model Context Protocol providers",
        Args:    nil,
    })
}
