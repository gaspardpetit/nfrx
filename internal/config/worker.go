package config

import (
	"flag"
	"strconv"
)

// WorkerConfig holds configuration for the worker agent.
type WorkerConfig struct {
	ServerURL      string
	Token          string
	OllamaURL      string
	MaxConcurrency int
	WorkerID       string
}

func (c *WorkerConfig) BindFlags() {
	c.ServerURL = getEnv("SERVER_URL", "ws://localhost:8080/workers/connect")
	c.Token = getEnv("TOKEN", "")
	c.OllamaURL = getEnv("OLLAMA_URL", "http://127.0.0.1:11434")
	mc := getEnv("MAX_CONCURRENCY", "2")
	if v, err := strconv.Atoi(mc); err == nil {
		c.MaxConcurrency = v
	} else {
		c.MaxConcurrency = 2
	}
	c.WorkerID = getEnv("WORKER_ID", "")

	flag.StringVar(&c.ServerURL, "server-url", c.ServerURL, "server websocket url")
	flag.StringVar(&c.Token, "token", c.Token, "registration token")
	flag.StringVar(&c.OllamaURL, "ollama-url", c.OllamaURL, "local Ollama URL")
	flag.IntVar(&c.MaxConcurrency, "max-concurrency", c.MaxConcurrency, "max concurrent jobs")
	flag.StringVar(&c.WorkerID, "worker-id", c.WorkerID, "worker identifier")
}
