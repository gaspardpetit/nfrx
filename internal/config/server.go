package config

import (
	"flag"
	"strconv"
	"time"
)

// ServerConfig holds configuration for the llamapool server.
type ServerConfig struct {
	Port           int
	APIKey         string
	WorkerKey      string
	WSPath         string
	RequestTimeout time.Duration
}

// BindFlags populates the struct with defaults from environment variables and
// binds command line flags so main can call flag.Parse().
func (c *ServerConfig) BindFlags() {
	port, _ := strconv.Atoi(getEnv("PORT", "8080"))
	c.Port = port
	c.APIKey = getEnv("API_KEY", "")
	c.WorkerKey = getEnv("WORKER_KEY", "")
	c.WSPath = getEnv("WS_PATH", "/workers/connect")
	rt, _ := time.ParseDuration(getEnv("REQUEST_TIMEOUT", "60s"))
	c.RequestTimeout = rt

	flag.IntVar(&c.Port, "port", c.Port, "HTTP listen port")
	flag.StringVar(&c.APIKey, "api-key", c.APIKey, "client API key")
	flag.StringVar(&c.WorkerKey, "worker-key", c.WorkerKey, "worker shared key")
	flag.StringVar(&c.WSPath, "ws-path", c.WSPath, "websocket path")
	flag.DurationVar(&c.RequestTimeout, "request-timeout", c.RequestTimeout, "request timeout")
}
