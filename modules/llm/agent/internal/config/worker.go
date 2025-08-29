package config

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	commoncfg "github.com/gaspardpetit/nfrx/core/config"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// WorkerConfig holds configuration for the worker agent.
type WorkerConfig struct {
	ServerURL          string
	ClientKey          string
	CompletionBaseURL  string
	CompletionAPIKey   string
	MaxConcurrency     int
	EmbeddingBatchSize int
	ClientID           string
	ClientName         string
	StatusAddr         string
	MetricsAddr        string
	DrainTimeout       time.Duration
	ModelPollInterval  time.Duration
	ConfigFile         string
	LogDir             string
	Reconnect          bool
	RequestTimeout     time.Duration
	LogLevel           string
}

func (c *WorkerConfig) BindFlags() {
	cfgPath, logDir := defaultWorkerPaths()
	c.ConfigFile = commoncfg.GetEnv("CONFIG_FILE", cfgPath)
	c.LogDir = commoncfg.GetEnv("LOG_DIR", logDir)
	c.LogLevel = commoncfg.GetEnv("LOG_LEVEL", "info")

	c.ServerURL = commoncfg.GetEnv("SERVER_URL", "ws://localhost:8080/api/llm/connect")
	c.ClientKey = commoncfg.GetEnv("CLIENT_KEY", "")
	base := commoncfg.GetEnv("COMPLETION_BASE_URL", "http://127.0.0.1:11434/v1")
	c.CompletionBaseURL = base
	c.CompletionAPIKey = commoncfg.GetEnv("COMPLETION_API_KEY", commoncfg.GetEnv("OLLAMA_API_KEY", ""))
	mc := commoncfg.GetEnv("MAX_CONCURRENCY", "2")
	if v, err := strconv.Atoi(mc); err == nil {
		c.MaxConcurrency = v
	} else {
		c.MaxConcurrency = 2
	}
	eb := commoncfg.GetEnv("EMBEDDING_BATCH_SIZE", "0")
	if v, err := strconv.Atoi(eb); err == nil {
		c.EmbeddingBatchSize = v
	}
	c.ClientID = commoncfg.GetEnv("CLIENT_ID", "")
	c.StatusAddr = commoncfg.GetEnv("STATUS_ADDR", "")
	mp := commoncfg.GetEnv("METRICS_PORT", "")
	if mp != "" && !strings.Contains(mp, ":") {
		mp = ":" + mp
	}
	c.MetricsAddr = mp
	if d, err := time.ParseDuration(commoncfg.GetEnv("DRAIN_TIMEOUT", "1m")); err == nil {
		c.DrainTimeout = d
	} else {
		c.DrainTimeout = time.Minute
	}
	if d, err := time.ParseDuration(commoncfg.GetEnv("MODEL_POLL_INTERVAL", "1m")); err == nil {
		c.ModelPollInterval = d
	} else {
		c.ModelPollInterval = time.Minute
	}
	if v, err := strconv.ParseFloat(commoncfg.GetEnv("REQUEST_TIMEOUT", "300"), 64); err == nil {
		c.RequestTimeout = time.Duration(v * float64(time.Second))
	} else {
		c.RequestTimeout = 5 * time.Minute
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "worker-" + uuid.NewString()[:8]
	}
	c.ClientName = commoncfg.GetEnv("CLIENT_NAME", host)
	if b, err := strconv.ParseBool(commoncfg.GetEnv("RECONNECT", "false")); err == nil {
		c.Reconnect = b
	}

	flag.StringVar(&c.ServerURL, "server-url", c.ServerURL, "server WebSocket URL for registration (e.g. ws://localhost:8080/api/llm/connect)")
	flag.StringVar(&c.ClientKey, "client-key", c.ClientKey, "shared secret for authenticating with the server")
	flag.StringVar(&c.CompletionBaseURL, "completion-base-url", c.CompletionBaseURL, "base URL of the completion API (e.g. http://127.0.0.1:11434/v1)")
	flag.StringVar(&c.CompletionAPIKey, "completion-api-key", c.CompletionAPIKey, "API key for the completion API; leave empty for no auth")
	flag.IntVar(&c.MaxConcurrency, "max-concurrency", c.MaxConcurrency, "maximum number of jobs processed concurrently")
	flag.IntVar(&c.EmbeddingBatchSize, "embedding-batch-size", c.EmbeddingBatchSize, "ideal embedding batch size for embeddings")
	flag.StringVar(&c.ClientID, "client-id", c.ClientID, "client identifier; randomly generated if omitted")
	flag.StringVar(&c.ClientName, "client-name", c.ClientName, "client display name shown in logs and status")
	flag.StringVar(&c.StatusAddr, "status-addr", c.StatusAddr, "local status HTTP listen address (enables /status; e.g. 127.0.0.1:4555)")
	flag.StringVar(&c.MetricsAddr, "metrics-port", c.MetricsAddr, "Prometheus metrics listen address or port (disabled when empty; e.g. 127.0.0.1:9090 or 9090)")
	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "worker config file path")
	flag.StringVar(&c.LogDir, "log-dir", c.LogDir, "directory for worker log files")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "log verbosity (all, debug, info, warn, error, fatal, none)")
	flag.DurationVar(&c.DrainTimeout, "drain-timeout", c.DrainTimeout, "time to wait for in-flight jobs on shutdown (-1 to wait indefinitely, 0 to exit immediately)")
	flag.DurationVar(&c.ModelPollInterval, "model-poll-interval", c.ModelPollInterval, "interval for polling backend for model changes")
	flag.Func("request-timeout", "request timeout in seconds without backend feedback", func(v string) error {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		c.RequestTimeout = time.Duration(f * float64(time.Second))
		return nil
	})
	flag.BoolVar(&c.Reconnect, "reconnect", c.Reconnect, "reconnect to server on failure")
	flag.BoolVar(&c.Reconnect, "r", c.Reconnect, "short for --reconnect")
}

func defaultWorkerPaths() (configFile, logDir string) {
	home, _ := os.UserHomeDir()
	programData := os.Getenv("ProgramData")
	return resolveWorkerPaths(runtime.GOOS, home, programData)
}

func resolveWorkerPaths(goos, home, programData string) (configFile, logDir string) {
	configFile = commoncfg.ResolveConfigPath(goos, home, programData, "worker.yaml")
	switch goos {
	case "darwin":
		logDir = filepath.Join(home, "Library", "Logs", "nfrx")
	case "windows":
		if programData == "" {
			programData = "C:/ProgramData"
		}
		programData = strings.TrimRight(programData, "\\/")
		logDir = filepath.Join(programData, "nfrx", "Logs")
	}
	return
}

// LoadFile populates the config from a YAML file. Fields already set remain unless
// overwritten by corresponding entries in the file.
func (c *WorkerConfig) LoadFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, c)
}
