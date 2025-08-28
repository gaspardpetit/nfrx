package spi

import "time"

// Options represents common server options available to all extensions.
// It includes global settings and a dictionary of per-plugin options.
type Options struct {
    // Global time to wait for worker activity before timing out a request.
    RequestTimeout time.Duration
    // Shared key clients must present when registering.
    ClientKey string
    // Maximum number of workers to split embeddings across (LLM-specific policy).
    // Kept here for now to maintain behavior; may move under plugin options later.
    MaxParallelEmbeddings int
    // PluginOptions holds extension-specific options keyed by plugin ID (e.g., "llm", "mcp").
    PluginOptions map[string]map[string]string
}

