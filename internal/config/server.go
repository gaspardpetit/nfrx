package config

import (
	"flag"
	"strconv"
	"time"
)

// ServerConfig holds configuration for the llamapool server.
type ServerConfig struct {
	Port           int
	WorkerToken    string
	WSPath         string
	RequestTimeout time.Duration
}

// BindFlags populates the struct with defaults from environment variables and
// binds command line flags so main can call flag.Parse().
func (c *ServerConfig) BindFlags() {
	port, _ := strconv.Atoi(getEnv("PORT", "8080"))
	c.Port = port
	c.WorkerToken = getEnv("WORKER_TOKEN", "")
	c.WSPath = getEnv("WS_PATH", "/workers/connect")
	rt, _ := time.ParseDuration(getEnv("REQUEST_TIMEOUT", "60s"))
	c.RequestTimeout = rt

	flag.IntVar(&c.Port, "port", c.Port, "HTTP listen port")
	flag.StringVar(&c.WorkerToken, "worker-token", c.WorkerToken, "worker shared token")
	flag.StringVar(&c.WSPath, "ws-path", c.WSPath, "websocket path")
	flag.DurationVar(&c.RequestTimeout, "request-timeout", c.RequestTimeout, "request timeout")
}
