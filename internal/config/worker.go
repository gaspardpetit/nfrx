package config

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WorkerConfig holds configuration for the worker agent.
type WorkerConfig struct {
	ServerURL         string
	ClientKey         string
	OllamaBaseURL     string
	OllamaAPIKey      string
	MaxConcurrency    int
	WorkerID          string
	WorkerName        string
	StatusAddr        string
	MetricsAddr       string
	DrainTimeout      time.Duration
	ModelPollInterval time.Duration
	ConfigFile        string
	LogDir            string
	Reconnect         bool
}

func (c *WorkerConfig) BindFlags() {
	cfgPath, logDir := defaultWorkerPaths()
	c.ConfigFile = getEnv("CONFIG_FILE", cfgPath)
	c.LogDir = getEnv("LOG_DIR", logDir)

	c.ServerURL = getEnv("SERVER_URL", "ws://localhost:8080/api/workers/connect")
	c.ClientKey = getEnv("CLIENT_KEY", "")
	base := getEnv("OLLAMA_BASE_URL", getEnv("OLLAMA_URL", "http://127.0.0.1:11434"))
	c.OllamaBaseURL = base
	c.OllamaAPIKey = getEnv("OLLAMA_API_KEY", "")
	mc := getEnv("MAX_CONCURRENCY", "2")
	if v, err := strconv.Atoi(mc); err == nil {
		c.MaxConcurrency = v
	} else {
		c.MaxConcurrency = 2
	}
	c.WorkerID = getEnv("WORKER_ID", "")
	c.StatusAddr = getEnv("STATUS_ADDR", "")
	mp := getEnv("METRICS_PORT", "")
	if mp != "" && !strings.Contains(mp, ":") {
		mp = ":" + mp
	}
	c.MetricsAddr = mp
	if d, err := time.ParseDuration(getEnv("DRAIN_TIMEOUT", "1m")); err == nil {
		c.DrainTimeout = d
	} else {
		c.DrainTimeout = time.Minute
	}
	if d, err := time.ParseDuration(getEnv("MODEL_POLL_INTERVAL", "1m")); err == nil {
		c.ModelPollInterval = d
	} else {
		c.ModelPollInterval = time.Minute
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "worker-" + uuid.NewString()[:8]
	}
	c.WorkerName = getEnv("WORKER_NAME", host)
	if b, err := strconv.ParseBool(getEnv("RECONNECT", "false")); err == nil {
		c.Reconnect = b
	}

	flag.StringVar(&c.ServerURL, "server-url", c.ServerURL, "server WebSocket URL for registration (e.g. ws://localhost:8080/api/workers/connect)")
	flag.StringVar(&c.ClientKey, "client-key", c.ClientKey, "shared secret for authenticating with the server")
	flag.StringVar(&c.OllamaBaseURL, "ollama-base-url", c.OllamaBaseURL, "base URL of the local Ollama instance (e.g. http://127.0.0.1:11434)")
	flag.StringVar(&c.OllamaAPIKey, "ollama-api-key", c.OllamaAPIKey, "API key for connecting to Ollama; leave empty for no auth")
	flag.IntVar(&c.MaxConcurrency, "max-concurrency", c.MaxConcurrency, "maximum number of jobs processed concurrently")
	flag.StringVar(&c.WorkerID, "worker-id", c.WorkerID, "worker identifier; randomly generated if omitted")
	flag.StringVar(&c.WorkerName, "worker-name", c.WorkerName, "worker display name shown in logs and status")
	flag.StringVar(&c.StatusAddr, "status-addr", c.StatusAddr, "local status HTTP listen address (enables /status; e.g. 127.0.0.1:4555)")
	flag.StringVar(&c.MetricsAddr, "metrics-port", c.MetricsAddr, "Prometheus metrics listen address or port (disabled when empty; e.g. 127.0.0.1:9090 or 9090)")
	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "worker config file path")
	flag.StringVar(&c.LogDir, "log-dir", c.LogDir, "directory for worker log files")
	flag.DurationVar(&c.DrainTimeout, "drain-timeout", c.DrainTimeout, "time to wait for in-flight jobs on shutdown (-1 to wait indefinitely, 0 to exit immediately)")
	flag.DurationVar(&c.ModelPollInterval, "model-poll-interval", c.ModelPollInterval, "interval for polling Ollama for model changes")
	flag.BoolVar(&c.Reconnect, "reconnect", c.Reconnect, "reconnect to server on failure")
	flag.BoolVar(&c.Reconnect, "r", c.Reconnect, "short for --reconnect")
}

func defaultWorkerPaths() (configFile, logDir string) {
	home, _ := os.UserHomeDir()
	programData := os.Getenv("ProgramData")
	return resolveWorkerPaths(runtime.GOOS, home, programData)
}

func resolveWorkerPaths(goos, home, programData string) (configFile, logDir string) {
	switch goos {
	case "darwin":
		configFile = filepath.Join(home, "Library", "Application Support", "llamapool", "worker.yaml")
		logDir = filepath.Join(home, "Library", "Logs", "llamapool")
	case "windows":
		if programData == "" {
			programData = "C:/ProgramData"
		}
		programData = strings.TrimRight(programData, "\\/")
		configFile = filepath.Join(programData, "llamapool", "worker.yaml")
		logDir = filepath.Join(programData, "llamapool", "Logs")
	default:
		// Linux and other platforms keep existing behavior with no defaults.
	}
	return
}
