package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	commoncfg "github.com/gaspardpetit/nfrx/core/config"
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	"gopkg.in/yaml.v3"
)

// ServerConfig holds configuration for the nfrx server.
type ServerConfig struct {
	Port           int
	MetricsAddr    string
	APIKey         string
	ClientKey      string
	RequestTimeout time.Duration
	DrainTimeout   time.Duration
	AllowedOrigins []string
	ConfigFile     string
	LogLevel       string
	RedisAddr      string
	Plugins        []string                     `yaml:"plugins"`
	PluginOptions  map[string]map[string]string `yaml:"plugin_options"`
}

// SetDefaults initializes c with built-in defaults.
func (c *ServerConfig) SetDefaults() {
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.Port == 0 {
		c.Port = 8080
	}
	if c.MetricsAddr == "" {
		c.MetricsAddr = fmt.Sprintf(":%d", c.Port)
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = 120 * time.Second
	}
	if c.DrainTimeout == 0 {
		c.DrainTimeout = 5 * time.Minute
	}
	if c.Plugins == nil {
		c.Plugins = []string{"*"}
	}
	if c.ConfigFile == "" {
		c.ConfigFile = commoncfg.DefaultConfigPath("server.yaml")
	}
}

// ApplyEnv overlays environment variables onto the current config values.
func (c *ServerConfig) ApplyEnv() {
	if v := commoncfg.GetEnv("CONFIG_FILE", ""); v != "" {
		c.ConfigFile = v
	}
	if v := commoncfg.GetEnv("LOG_LEVEL", ""); v != "" {
		c.LogLevel = v
	}
	if v := commoncfg.GetEnv("PORT", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Port = n
		}
	}
	if v := commoncfg.GetEnv("METRICS_PORT", ""); v != "" {
		if strings.Contains(v, ":") {
			c.MetricsAddr = v
		} else {
			c.MetricsAddr = ":" + v
		}
	} else if c.MetricsAddr == "" {
		c.MetricsAddr = fmt.Sprintf(":%d", c.Port)
	}
	if v := commoncfg.GetEnv("API_KEY", ""); v != "" {
		c.APIKey = v
	}
	if v := commoncfg.GetEnv("CLIENT_KEY", ""); v != "" {
		c.ClientKey = v
	}
	if v := commoncfg.GetEnv("REDIS_ADDR", ""); v != "" {
		c.RedisAddr = v
	}
	if v := commoncfg.GetEnv("REQUEST_TIMEOUT", ""); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.RequestTimeout = time.Duration(f * float64(time.Second))
		}
	}
	if v := commoncfg.GetEnv("DRAIN_TIMEOUT", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.DrainTimeout = d
		}
	}
	if v := commoncfg.GetEnv("ALLOWED_ORIGINS", ""); v != "" {
		c.AllowedOrigins = splitComma(v)
	}
	if v := commoncfg.GetEnv("PLUGINS", ""); v != "" {
		c.Plugins = splitComma(v)
	}
}

// BindFlagsFromCurrent binds command line flags using the current config values as defaults.
func (c *ServerConfig) BindFlagsFromCurrent() {
	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "server config file path")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "log verbosity (all, debug, info, warn, error, fatal, none)")
	flag.IntVar(&c.Port, "port", c.Port, "HTTP listen port for the public API")
	flag.StringVar(&c.MetricsAddr, "metrics-port", c.MetricsAddr, "Prometheus metrics listen address or port; defaults to the value of --port")
	flag.StringVar(&c.APIKey, "api-key", c.APIKey, "client API key required for HTTP requests; leave empty to disable auth")
	flag.StringVar(&c.ClientKey, "client-key", c.ClientKey, "shared key clients must present when registering")
	flag.StringVar(&c.RedisAddr, "redis-addr", c.RedisAddr, "redis connection URL for server state")
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

// SetPluginOption sets an extension option value under plugin_options.<pluginID>.<key>.
func (c *ServerConfig) SetPluginOption(pluginID, key, value string) {
	if c.PluginOptions == nil {
		c.PluginOptions = map[string]map[string]string{}
	}
	po := c.PluginOptions[pluginID]
	if po == nil {
		po = map[string]string{}
	}
	po[key] = value
	c.PluginOptions[pluginID] = po
}

// ApplyEnvExtensions overlays extension options from environment variables based on descriptors.
// For each ArgSpec with an Env, if the env var is set, the value is stored under plugin_options.
func (c *ServerConfig) ApplyEnvExtensions(descs map[string]spi.PluginDescriptor) {
	if c.PluginOptions == nil {
		c.PluginOptions = map[string]map[string]string{}
	}
	for id, d := range descs {
		for _, a := range d.Args {
			if a.Env == "" {
				continue
			}
			if v := os.Getenv(a.Env); v != "" {
				po := c.PluginOptions[id]
				if po == nil {
					po = map[string]string{}
				}
				po[a.ID] = v
				c.PluginOptions[id] = po
			}
		}
	}
}
