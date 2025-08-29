package workerproxy

import "time"

// Config holds the settings for the generic worker HTTP-proxy agent.
type Config struct {
	// Control-plane connection
	ServerURL string
	ClientKey string

	// Upstream service
	BaseURL string
	APIKey  string

	// Health probe
	ProbePath string // e.g. "/health"; empty disables periodic probe

	// Concurrency and identity
	MaxConcurrency int
	ClientID       string
	ClientName     string

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
