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
	ServerURL      string
	WorkerKey      string
	OllamaBaseURL  string
	OllamaAPIKey   string
	MaxConcurrency int
	WorkerID       string
	WorkerName     string
	StatusAddr     string
	DrainTimeout   time.Duration
	ConfigFile     string
	LogDir         string
}

func (c *WorkerConfig) BindFlags() {
	cfgPath, logDir := defaultWorkerPaths()
	c.ConfigFile = getEnv("CONFIG_FILE", cfgPath)
	c.LogDir = getEnv("LOG_DIR", logDir)

	c.ServerURL = getEnv("SERVER_URL", "ws://localhost:8080/workers/connect")
	c.WorkerKey = getEnv("WORKER_KEY", "")
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
	if d, err := time.ParseDuration(getEnv("DRAIN_TIMEOUT", "1m")); err == nil {
		c.DrainTimeout = d
	} else {
		c.DrainTimeout = time.Minute
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "worker-" + uuid.NewString()[:8]
	}
	c.WorkerName = getEnv("WORKER_NAME", host)

	flag.StringVar(&c.ServerURL, "server-url", c.ServerURL, "server websocket url")
	flag.StringVar(&c.WorkerKey, "worker-key", c.WorkerKey, "worker auth key")
	flag.StringVar(&c.OllamaBaseURL, "ollama-base-url", c.OllamaBaseURL, "base URL for local Ollama")
	flag.StringVar(&c.OllamaAPIKey, "ollama-api-key", c.OllamaAPIKey, "Ollama API key")
	flag.IntVar(&c.MaxConcurrency, "max-concurrency", c.MaxConcurrency, "max concurrent jobs")
	flag.StringVar(&c.WorkerID, "worker-id", c.WorkerID, "worker identifier")
	flag.StringVar(&c.WorkerName, "worker-name", c.WorkerName, "worker display name")
	flag.StringVar(&c.StatusAddr, "status-addr", c.StatusAddr, "local status http listen address")
	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "worker config file path")
	flag.StringVar(&c.LogDir, "log-dir", c.LogDir, "log directory")
	flag.DurationVar(&c.DrainTimeout, "drain-timeout", c.DrainTimeout, "time to wait for in-flight jobs on shutdown (-1 to wait indefinitely, 0 to exit immediately)")
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
