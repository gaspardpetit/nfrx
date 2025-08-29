package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	commoncfg "github.com/gaspardpetit/nfrx/core/config"
)

// MCPConfig holds configuration for the MCP relay.
type MCPConfig struct {
	ServerURL      string
	ClientKey      string
	ClientID       string
	ClientName     string
	ProviderURL    string
	AuthToken      string
	MetricsAddr    string
	RequestTimeout time.Duration
	Reconnect      bool
	ConfigFile     string
	LogLevel       string
}

// BindFlags populates the struct with defaults from environment variables and
// binds command line flags so main can call flag.Parse().
func (c *MCPConfig) BindFlags() {
	cfgPath := commoncfg.DefaultConfigPath("mcp.yaml")
	c.ConfigFile = commoncfg.GetEnv("CONFIG_FILE", cfgPath)
	c.LogLevel = commoncfg.GetEnv("LOG_LEVEL", "info")

	c.ServerURL = commoncfg.GetEnv("SERVER_URL", "ws://localhost:8080/api/mcp/connect")
	c.ClientKey = commoncfg.GetEnv("CLIENT_KEY", "")
	c.ProviderURL = commoncfg.GetEnv("PROVIDER_URL", "http://127.0.0.1:7777/")
	c.AuthToken = commoncfg.GetEnv("AUTH_TOKEN", "")
	mp := commoncfg.GetEnv("METRICS_PORT", "")
	if mp != "" && !strings.Contains(mp, ":") {
		mp = ":" + mp
	}
	c.MetricsAddr = mp
	if v, err := strconv.ParseFloat(commoncfg.GetEnv("REQUEST_TIMEOUT", "300"), 64); err == nil {
		c.RequestTimeout = time.Duration(v * float64(time.Second))
	} else {
		c.RequestTimeout = 5 * time.Minute
	}
	c.ClientID = commoncfg.GetEnv("CLIENT_ID", "")
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "mcp-" + uuid.NewString()[:8]
	}
	c.ClientName = commoncfg.GetEnv("CLIENT_NAME", host)
	if b, err := strconv.ParseBool(commoncfg.GetEnv("RECONNECT", "false")); err == nil {
		c.Reconnect = b
	}

	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "mcp config file path")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "log verbosity (all, debug, info, warn, error, fatal, none)")
	flag.StringVar(&c.ServerURL, "server-url", c.ServerURL, "broker WebSocket URL (e.g. ws://localhost:8080/api/mcp/connect)")
	flag.StringVar(&c.ClientKey, "client-key", c.ClientKey, "shared secret for authenticating with the server")
	flag.StringVar(&c.ProviderURL, "provider-url", c.ProviderURL, "MCP provider URL")
	flag.StringVar(&c.AuthToken, "auth-token", c.AuthToken, "authorization token for broker requests")
	flag.StringVar(&c.MetricsAddr, "metrics-port", c.MetricsAddr, "Prometheus metrics listen address or port (disabled when empty; e.g. 127.0.0.1:9090 or 9090)")
	flag.Func("request-timeout", "request timeout in seconds for provider responses", func(v string) error {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		c.RequestTimeout = time.Duration(f * float64(time.Second))
		return nil
	})
	flag.StringVar(&c.ClientID, "client-id", c.ClientID, "client identifier; assigned when empty")
	flag.StringVar(&c.ClientName, "client-name", c.ClientName, "client display name shown in logs and status")
	flag.BoolVar(&c.Reconnect, "reconnect", c.Reconnect, "reconnect to server on failure")
	flag.BoolVar(&c.Reconnect, "r", c.Reconnect, "short for --reconnect")
}

// LoadFile populates the config from a YAML file. Fields already set remain unless
// overwritten by corresponding entries in the file.
func (c *MCPConfig) LoadFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, c)
}
