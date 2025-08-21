package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig holds configuration for the llamapool server.
type ServerConfig struct {
	Port           int
	MetricsPort    int
	APIKey         string
	ClientKey      string
	RequestTimeout time.Duration
	AllowedOrigins []string
	ConfigFile     string
}

// BindFlags populates the struct with defaults from environment variables and
// binds command line flags so main can call flag.Parse().
func (c *ServerConfig) BindFlags() {
	cfgPath := DefaultConfigPath("server.yaml")
	c.ConfigFile = getEnv("CONFIG_FILE", cfgPath)

	port, _ := strconv.Atoi(getEnv("PORT", "8080"))
	c.Port = port
	mp, _ := strconv.Atoi(getEnv("METRICS_PORT", strconv.Itoa(port)))
	c.MetricsPort = mp
	c.APIKey = getEnv("API_KEY", "")
	c.ClientKey = getEnv("CLIENT_KEY", "")
	rt, _ := time.ParseDuration(getEnv("REQUEST_TIMEOUT", "60s"))
	c.RequestTimeout = rt
	c.AllowedOrigins = splitComma(getEnv("ALLOWED_ORIGINS", strings.Join(c.AllowedOrigins, ",")))

	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "server config file path")
	flag.IntVar(&c.Port, "port", c.Port, "HTTP listen port for the public API")
	flag.IntVar(&c.MetricsPort, "metrics-port", c.MetricsPort, "Prometheus metrics listen port; defaults to the value of --port")
	flag.StringVar(&c.APIKey, "api-key", c.APIKey, "client API key required for HTTP requests; leave empty to disable auth")
	flag.StringVar(&c.ClientKey, "client-key", c.ClientKey, "shared key clients must present when registering")
	flag.DurationVar(&c.RequestTimeout, "request-timeout", c.RequestTimeout, "maximum duration to process a client request")
	flag.Func("allowed-origins", "comma separated list of allowed CORS origins", func(v string) error {
		c.AllowedOrigins = splitComma(v)
		return nil
	})
}

func splitComma(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

// LoadFile populates the config from a YAML file.
func (c *ServerConfig) LoadFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, c)
}
