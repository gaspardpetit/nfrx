package workerproxy

import (
	"context"
	"time"
)

// Config holds the settings for the generic worker HTTP-proxy agent.
type Config struct {
	// Control-plane connection
	ServerURL string
	ClientKey string

	// Upstream service
	BaseURL string
	APIKey  string

	// Health probe
	// ProbeFunc is responsible for returning readiness information, including
	// optional model metadata for schedulers. When nil, the agent assumes the
	// backend is ready at startup and advertises the configured MaxConcurrency
	// without probing.
	ProbeFunc     ProbeFunc
	ProbeInterval time.Duration // when zero, defaults to 20s

	// Concurrency and identity
	MaxConcurrency int
	ClientID       string
	ClientName     string
	// Optional extra config sent to the server via AgentConfig (e.g., embedding_batch_size)
	AgentConfig map[string]string

	// Local servers
	StatusAddr    string        // status + drain control HTTP server (optional)
	MetricsAddr   string        // Prometheus metrics listen address or port (optional)
	TokenBasename string        // basename for the drain token file (default: "agent")
	DrainTimeout  time.Duration // drain shutdown timeout

	// Timeouts and behavior
	RequestTimeout time.Duration
	Reconnect      bool

	// Optional: path of config file to co-locate token file
	ConfigFile string
}

// ProbeResult reports backend readiness and optional scheduling metadata.
type ProbeResult struct {
	Ready          bool
	Models         []string
	MaxConcurrency int
}

// ProbeFunc queries the upstream backend for readiness and metadata.
type ProbeFunc func(ctx context.Context) (ProbeResult, error)
