package config

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ServerConfig holds configuration for the llamapool server.
type ServerConfig struct {
	Port           int
	MetricsAddr    string
	APIKey         string
	ClientKey      string
	RequestTimeout time.Duration
	AllowedOrigins []string
}

// BindFlags populates the struct with defaults from environment variables and
// binds command line flags so main can call flag.Parse().
func (c *ServerConfig) BindFlags() {
	port, _ := strconv.Atoi(getEnv("PORT", "8080"))
	c.Port = port
	mp := getEnv("METRICS_PORT", "")
	if mp == "" {
		c.MetricsAddr = fmt.Sprintf(":%d", port)
	} else if strings.Contains(mp, ":") {
		c.MetricsAddr = mp
	} else {
		c.MetricsAddr = ":" + mp
	}
	c.APIKey = getEnv("API_KEY", "")
	c.ClientKey = getEnv("CLIENT_KEY", "")
	rt, _ := time.ParseDuration(getEnv("REQUEST_TIMEOUT", "60s"))
	c.RequestTimeout = rt
	c.AllowedOrigins = splitComma(getEnv("ALLOWED_ORIGINS", strings.Join(c.AllowedOrigins, ",")))

	flag.IntVar(&c.Port, "port", c.Port, "HTTP listen port for the public API")
	flag.StringVar(&c.MetricsAddr, "metrics-port", c.MetricsAddr, "Prometheus metrics listen address or port; defaults to the value of --port")
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
