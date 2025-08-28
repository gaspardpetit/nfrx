package spi

import "time"

// Options represents common server options available to all extensions.
// It includes global settings and a dictionary of per-plugin options.
type Options struct {
    // Global time to wait for worker activity before timing out a request.
    RequestTimeout time.Duration
    // Shared key clients must present when registering.
    ClientKey string
    // AgentHeartbeatInterval controls how often connected agents are expected to send heartbeats.
    // If zero, defaults are used by the server.
    AgentHeartbeatInterval time.Duration
    // AgentHeartbeatExpiry controls how long the server waits without a heartbeat before evicting an agent.
    // If zero, defaults are used by the server.
    AgentHeartbeatExpiry time.Duration
    // PluginOptions holds extension-specific options keyed by plugin ID (e.g., "llm", "mcp").
    PluginOptions map[string]map[string]string
}
