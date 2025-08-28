package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	commoncfg "github.com/gaspardpetit/nfrx/modules/common/config"
	"gopkg.in/yaml.v3"
)

// ServerConfig holds configuration for the nfrx server.
type ServerConfig struct {
	Port                  int
	MetricsAddr           string
	APIKey                string
	ClientKey             string
	RequestTimeout        time.Duration
	DrainTimeout          time.Duration
	AllowedOrigins        []string
	ConfigFile            string
	LogLevel              string
	RedisAddr             string
	MaxParallelEmbeddings int
	Plugins               []string                     `yaml:"plugins"`
	PluginOptions         map[string]map[string]string `yaml:"plugin_options"`
}

// BindFlags populates the struct with defaults from environment variables and
// binds command line flags so main can call flag.Parse().
func (c *ServerConfig) BindFlags() {
	cfgPath := commoncfg.DefaultConfigPath("server.yaml")
	c.ConfigFile = commoncfg.GetEnv("CONFIG_FILE", cfgPath)
	c.LogLevel = commoncfg.GetEnv("LOG_LEVEL", "info")

	port, _ := strconv.Atoi(commoncfg.GetEnv("PORT", "8080"))
	c.Port = port
	mp := commoncfg.GetEnv("METRICS_PORT", "")
	if mp == "" {
		c.MetricsAddr = fmt.Sprintf(":%d", port)
	} else if strings.Contains(mp, ":") {
		c.MetricsAddr = mp
	} else {
		c.MetricsAddr = ":" + mp
	}
	c.APIKey = commoncfg.GetEnv("API_KEY", "")
	c.ClientKey = commoncfg.GetEnv("CLIENT_KEY", "")
	c.RedisAddr = commoncfg.GetEnv("REDIS_ADDR", "")
	if v, err := strconv.Atoi(commoncfg.GetEnv("MAX_PARALLEL_EMBEDDINGS", "8")); err == nil {
		c.MaxParallelEmbeddings = v
	} else {
		c.MaxParallelEmbeddings = 8
	}
	if v, err := strconv.ParseFloat(commoncfg.GetEnv("REQUEST_TIMEOUT", "120"), 64); err == nil {
		c.RequestTimeout = time.Duration(v * float64(time.Second))
	} else {
		c.RequestTimeout = 120 * time.Second
	}
	if d, err := time.ParseDuration(commoncfg.GetEnv("DRAIN_TIMEOUT", "5m")); err == nil {
		c.DrainTimeout = d
	} else {
		c.DrainTimeout = 5 * time.Minute
	}
	c.AllowedOrigins = splitComma(commoncfg.GetEnv("ALLOWED_ORIGINS", strings.Join(c.AllowedOrigins, ",")))
    if p := commoncfg.GetEnv("PLUGINS", ""); p != "" {
        c.Plugins = splitComma(p)
    } else if c.Plugins == nil {
        // Default to loading all registered plugins
        c.Plugins = []string{"*"}
    }

	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "server config file path")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "log verbosity (all, debug, info, warn, error, fatal, none)")
	flag.IntVar(&c.Port, "port", c.Port, "HTTP listen port for the public API")
	flag.StringVar(&c.MetricsAddr, "metrics-port", c.MetricsAddr, "Prometheus metrics listen address or port; defaults to the value of --port")
	flag.StringVar(&c.APIKey, "api-key", c.APIKey, "client API key required for HTTP requests; leave empty to disable auth")
	flag.StringVar(&c.ClientKey, "client-key", c.ClientKey, "shared key clients must present when registering")
	flag.StringVar(&c.RedisAddr, "redis-addr", c.RedisAddr, "redis connection URL for server state")
	flag.IntVar(&c.MaxParallelEmbeddings, "max-parallel-embeddings", c.MaxParallelEmbeddings, "maximum number of workers to split embeddings across")
	flag.Func("plugins", "comma separated list of enabled plugins", func(v string) error {
		c.Plugins = splitComma(v)
		return nil
	})
	flag.Func("request-timeout", "request timeout in seconds without worker activity", func(v string) error {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return err
		}
		c.RequestTimeout = time.Duration(f * float64(time.Second))
		return nil
	})
	flag.DurationVar(&c.DrainTimeout, "drain-timeout", c.DrainTimeout, "time to wait for in-flight requests on shutdown (-1 to wait indefinitely, 0 to exit immediately)")
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
