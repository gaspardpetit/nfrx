package mcp

import "github.com/gaspardpetit/nfrx/sdk/api/spi"

// Descriptor returns the MCP plugin descriptor.
func Descriptor() spi.PluginDescriptor {
	return spi.PluginDescriptor{
		ID:      "mcp",
		Name:    "MCP Relay",
		Summary: "Relay for Model Context Protocol providers",
		Args: []spi.ArgSpec{
			{ID: "max_req_bytes", Flag: "--mcp-max-req-bytes", Env: "BROKER_MAX_REQ_BYTES", YAML: "plugin_options.mcp.max_req_bytes", Type: spi.ArgInt, Default: "10485760", Example: "20971520", Description: "Maximum MCP request size in bytes"},
			{ID: "max_resp_bytes", Flag: "--mcp-max-resp-bytes", Env: "BROKER_MAX_RESP_BYTES", YAML: "plugin_options.mcp.max_resp_bytes", Type: spi.ArgInt, Default: "10485760", Example: "20971520", Description: "Maximum MCP response size in bytes"},
			{ID: "ws_heartbeat_ms", Flag: "--mcp-ws-heartbeat-ms", Env: "BROKER_WS_HEARTBEAT_MS", YAML: "plugin_options.mcp.ws_heartbeat_ms", Type: spi.ArgInt, Default: "15000", Example: "10000", Description: "Ping interval to connected MCP relays (milliseconds)"},
			{ID: "ws_dead_after_ms", Flag: "--mcp-ws-dead-after-ms", Env: "BROKER_WS_DEAD_AFTER_MS", YAML: "plugin_options.mcp.ws_dead_after_ms", Type: spi.ArgInt, Default: "45000", Example: "30000", Description: "Disconnect MCP relay if no pong within this period (milliseconds)"},
			{ID: "max_concurrency_per_client", Flag: "--mcp-max-concurrency-per-client", Env: "BROKER_MAX_CONCURRENCY_PER_CLIENT", YAML: "plugin_options.mcp.max_concurrency_per_client", Type: spi.ArgInt, Default: "16", Example: "32", Description: "Maximum concurrent MCP sessions per client"},
		},
	}
}
