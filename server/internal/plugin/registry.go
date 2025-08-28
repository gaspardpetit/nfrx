package plugin

import (
    "net/http"

    llm "github.com/gaspardpetit/nfrx/modules/llm/ext"
    mcp "github.com/gaspardpetit/nfrx/modules/mcp/ext"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
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
        Args: []spi.ArgSpec{
            {
                ID:          "max_req_bytes",
                Flag:        "--mcp-max-req-bytes",
                Env:         "BROKER_MAX_REQ_BYTES",
                YAML:        "plugin_options.mcp.max_req_bytes",
                Type:        spi.ArgInt,
                Default:     "10485760",
                Example:     "20971520",
                Description: "Maximum MCP request size in bytes",
            },
            {
                ID:          "max_resp_bytes",
                Flag:        "--mcp-max-resp-bytes",
                Env:         "BROKER_MAX_RESP_BYTES",
                YAML:        "plugin_options.mcp.max_resp_bytes",
                Type:        spi.ArgInt,
                Default:     "10485760",
                Example:     "20971520",
                Description: "Maximum MCP response size in bytes",
            },
            {
                ID:          "ws_heartbeat_ms",
                Flag:        "--mcp-ws-heartbeat-ms",
                Env:         "BROKER_WS_HEARTBEAT_MS",
                YAML:        "plugin_options.mcp.ws_heartbeat_ms",
                Type:        spi.ArgInt,
                Default:     "15000",
                Example:     "10000",
                Description: "Ping interval to connected MCP relays (milliseconds)",
            },
            {
                ID:          "ws_dead_after_ms",
                Flag:        "--mcp-ws-dead-after-ms",
                Env:         "BROKER_WS_DEAD_AFTER_MS",
                YAML:        "plugin_options.mcp.ws_dead_after_ms",
                Type:        spi.ArgInt,
                Default:     "45000",
                Example:     "30000",
                Description: "Disconnect MCP relay if no pong within this period (milliseconds)",
            },
            {
                ID:          "max_concurrency_per_client",
                Flag:        "--mcp-max-concurrency-per-client",
                Env:         "BROKER_MAX_CONCURRENCY_PER_CLIENT",
                YAML:        "plugin_options.mcp.max_concurrency_per_client",
                Type:        spi.ArgInt,
                Default:     "16",
                Example:     "32",
                Description: "Maximum concurrent MCP sessions per client",
            },
        },
    })
}
