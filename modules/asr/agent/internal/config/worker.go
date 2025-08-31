package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	commoncfg "github.com/gaspardpetit/nfrx/core/config"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type WorkerConfig struct {
	ServerURL      string
	ClientKey      string
	BaseURL        string
	APIKey         string
	MaxConcurrency int
	ClientID       string
	ClientName     string
	StatusAddr     string
	MetricsAddr    string
	DrainTimeout   time.Duration
	RequestTimeout time.Duration
	ConfigFile     string
	LogLevel       string
	Reconnect      bool
}

func (c *WorkerConfig) BindFlags() {
	cfgPath := commoncfg.DefaultConfigPath("asr.yaml")
	c.ConfigFile = commoncfg.GetEnv("CONFIG_FILE", cfgPath)
	c.LogLevel = commoncfg.GetEnv("LOG_LEVEL", "info")

	c.ServerURL = commoncfg.GetEnv("SERVER_URL", "ws://localhost:8080/api/asr/connect")
	c.ClientKey = commoncfg.GetEnv("CLIENT_KEY", "")
	c.BaseURL = commoncfg.GetEnv("ASR_BASE_URL", "http://127.0.0.1:5002")
	c.APIKey = commoncfg.GetEnv("ASR_API_KEY", "")
	if v, err := strconv.Atoi(commoncfg.GetEnv("MAX_CONCURRENCY", "2")); err == nil {
		c.MaxConcurrency = v
	} else {
		c.MaxConcurrency = 2
	}
	c.ClientID = commoncfg.GetEnv("CLIENT_ID", "")
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
	if v, err := strconv.ParseFloat(commoncfg.GetEnv("REQUEST_TIMEOUT", "300"), 64); err == nil {
		c.RequestTimeout = time.Duration(v * float64(time.Second))
	} else {
		c.RequestTimeout = 5 * time.Minute
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "asr-" + uuid.NewString()[:8]
	}
	c.ClientName = commoncfg.GetEnv("CLIENT_NAME", host)
	if b, err := strconv.ParseBool(commoncfg.GetEnv("RECONNECT", "false")); err == nil {
		c.Reconnect = b
	}

	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "agent config file path")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "log verbosity")
	flag.StringVar(&c.ServerURL, "server-url", c.ServerURL, "server WebSocket URL")
	flag.StringVar(&c.ClientKey, "client-key", c.ClientKey, "shared secret with server")
	flag.StringVar(&c.BaseURL, "asr-base-url", c.BaseURL, "ASR service base URL")
	flag.StringVar(&c.APIKey, "asr-api-key", c.APIKey, "ASR API key for Authorization bearer")
	flag.IntVar(&c.MaxConcurrency, "max-concurrency", c.MaxConcurrency, "max concurrent jobs")
	flag.StringVar(&c.ClientID, "client-id", c.ClientID, "client identifier")
	flag.StringVar(&c.ClientName, "client-name", c.ClientName, "client display name")
	flag.StringVar(&c.StatusAddr, "status-addr", c.StatusAddr, "local status HTTP listen address")
	flag.StringVar(&c.MetricsAddr, "metrics-port", c.MetricsAddr, "Prometheus metrics listen address or port")
	flag.DurationVar(&c.DrainTimeout, "drain-timeout", c.DrainTimeout, "shutdown drain timeout")
	flag.Func("request-timeout", "request timeout in seconds", func(v string) error {
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

func (c *WorkerConfig) LoadFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, c)
}
