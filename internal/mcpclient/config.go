package mcpclient

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gaspardpetit/infero/internal/config"
	"github.com/mark3labs/mcp-go/client/transport"
	"gopkg.in/yaml.v3"
)

// Config controls how the MCP client connects to third-party servers.
type Config struct {
	// ConfigFile optionally points to a YAML file loaded before env/flags.
	ConfigFile string

	// Order defines the ordered list of transports to attempt.
	// Valid entries: "stdio", "http", "oauth", "legacy-sse".
	Order []string

	// InitTimeout caps the time allowed for each transport to start and initialize.
	InitTimeout time.Duration

	// ProtocolVersion is the preferred MCP protocol version to negotiate.
	ProtocolVersion string

	// MaxInFlight bounds concurrent RPC requests. Zero disables the limit.
	MaxInFlight int

	// Stdio holds settings for the stdio transport.
	Stdio StdioConfig

	// HTTP holds settings for HTTP based transports.
	HTTP HTTPConfig

	// OAuth holds optional OAuth settings for HTTP transports.
	OAuth OAuthConfig

	// EnableLegacySSE gates the legacy SSE transport.
	EnableLegacySSE bool
}

// StdioConfig describes how to spawn a local MCP server over stdio.
type StdioConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Env     []string `yaml:"env"`
	WorkDir string   `yaml:"workDir"`
	// AllowRelative permits non-absolute command paths when true.
	AllowRelative bool `yaml:"allowRelative"`
}

// HTTPConfig describes a remote HTTP MCP server.
type HTTPConfig struct {
	URL string

	// Timeout controls connect/read/write timeouts for the HTTP client.
	Timeout time.Duration

	// EnablePush opens a background SSE channel when supported.
	EnablePush bool
	// InsecureSkipVerify disables TLS certificate verification when true.
	InsecureSkipVerify bool `yaml:"insecureSkipVerify"`
}

// OAuthConfig contains optional OAuth parameters.
type OAuthConfig struct {
	Enabled      bool
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
	TokenStore   transport.TokenStore
	// TokenFile optionally caches OAuth tokens on disk with 0600 permissions.
	TokenFile string `yaml:"tokenFile"`
}

// BindFlags populates the config using environment variables and binds CLI flags.
func (c *Config) BindFlags() {
	if len(c.Order) == 0 {
		c.Order = []string{"stdio", "http", "oauth"}
	}
	if c.InitTimeout == 0 {
		c.InitTimeout = 5 * time.Second
	}
	if c.HTTP.Timeout == 0 {
		c.HTTP.Timeout = 30 * time.Second
	}
	cfgPath := config.DefaultConfigPath("mcp.yaml")
	c.ConfigFile = getEnv("CONFIG_FILE", cfgPath)
	c.Order = splitComma(getEnv("MCP_TRANSPORT_ORDER", strings.Join(c.Order, ",")))
	c.InitTimeout = parseDuration(getEnv("MCP_INIT_TIMEOUT", c.InitTimeout.String()))
	c.ProtocolVersion = getEnv("MCP_PROTOCOL_VERSION", c.ProtocolVersion)
	c.MaxInFlight = parseInt(getEnv("MCP_MAX_INFLIGHT", strconv.Itoa(c.MaxInFlight)))

	c.Stdio.Command = getEnv("MCP_STDIO_COMMAND", c.Stdio.Command)
	c.Stdio.Args = splitComma(getEnv("MCP_STDIO_ARGS", strings.Join(c.Stdio.Args, ",")))
	c.Stdio.Env = splitComma(getEnv("MCP_STDIO_ENV", strings.Join(c.Stdio.Env, ",")))
	c.Stdio.WorkDir = getEnv("MCP_STDIO_WORKDIR", c.Stdio.WorkDir)
	if getEnv("MCP_STDIO_ALLOW_RELATIVE", "") != "" {
		c.Stdio.AllowRelative = getEnv("MCP_STDIO_ALLOW_RELATIVE", "") == "true"
	}

	c.HTTP.URL = getEnv("MCP_HTTP_URL", c.HTTP.URL)
	c.HTTP.Timeout = parseDuration(getEnv("MCP_HTTP_TIMEOUT", c.HTTP.Timeout.String()))
	if getEnv("MCP_HTTP_ENABLE_PUSH", "") != "" {
		c.HTTP.EnablePush = getEnv("MCP_HTTP_ENABLE_PUSH", "") == "true"
	}
	if getEnv("MCP_HTTP_INSECURE_SKIP_VERIFY", "") != "" {
		c.HTTP.InsecureSkipVerify = getEnv("MCP_HTTP_INSECURE_SKIP_VERIFY", "") == "true"
	}

	if getEnv("MCP_OAUTH_ENABLED", "") != "" {
		c.OAuth.Enabled = getEnv("MCP_OAUTH_ENABLED", "") == "true"
	}
	c.OAuth.TokenURL = getEnv("MCP_OAUTH_TOKEN_URL", c.OAuth.TokenURL)
	c.OAuth.ClientID = getEnv("MCP_OAUTH_CLIENT_ID", c.OAuth.ClientID)
	c.OAuth.ClientSecret = getEnv("MCP_OAUTH_CLIENT_SECRET", c.OAuth.ClientSecret)
	c.OAuth.Scopes = splitComma(getEnv("MCP_OAUTH_SCOPES", strings.Join(c.OAuth.Scopes, ",")))
	c.OAuth.TokenFile = getEnv("MCP_OAUTH_TOKEN_FILE", c.OAuth.TokenFile)

	if getEnv("MCP_ENABLE_LEGACY_SSE", "") != "" {
		c.EnableLegacySSE = getEnv("MCP_ENABLE_LEGACY_SSE", "") == "true"
	}

	flag.StringVar(&c.ConfigFile, "config", c.ConfigFile, "path to YAML config file")
	flag.Func("mcp-transport-order", "comma separated transport order", func(v string) error { c.Order = splitComma(v); return nil })
	flag.DurationVar(&c.InitTimeout, "mcp-init-timeout", c.InitTimeout, "timeout for transport startup and initialization")
	flag.StringVar(&c.ProtocolVersion, "mcp-protocol-version", c.ProtocolVersion, "preferred MCP protocol version")
	flag.IntVar(&c.MaxInFlight, "mcp-max-inflight", c.MaxInFlight, "maximum concurrent MCP RPCs")
	flag.StringVar(&c.Stdio.Command, "mcp-stdio-command", c.Stdio.Command, "command for stdio transport")
	flag.Var(newCSVValue(c.Stdio.Args, &c.Stdio.Args), "mcp-stdio-args", "stdio command arguments")
	flag.Var(newCSVValue(c.Stdio.Env, &c.Stdio.Env), "mcp-stdio-env", "stdio environment variables")
	flag.StringVar(&c.Stdio.WorkDir, "mcp-stdio-workdir", c.Stdio.WorkDir, "stdio working directory")
	flag.BoolVar(&c.Stdio.AllowRelative, "mcp-stdio-allow-relative", c.Stdio.AllowRelative, "allow relative stdio command path")
	flag.StringVar(&c.HTTP.URL, "mcp-http-url", c.HTTP.URL, "HTTP MCP server base URL")
	flag.DurationVar(&c.HTTP.Timeout, "mcp-http-timeout", c.HTTP.Timeout, "HTTP client timeout")
	flag.BoolVar(&c.HTTP.EnablePush, "mcp-http-enable-push", c.HTTP.EnablePush, "enable server-push SSE channel")
	flag.BoolVar(&c.HTTP.InsecureSkipVerify, "mcp-http-insecure-skip-verify", c.HTTP.InsecureSkipVerify, "skip TLS certificate verification (insecure)")
	flag.BoolVar(&c.OAuth.Enabled, "mcp-oauth-enabled", c.OAuth.Enabled, "enable OAuth for HTTP transport")
	flag.StringVar(&c.OAuth.TokenURL, "mcp-oauth-token-url", c.OAuth.TokenURL, "OAuth token endpoint")
	flag.StringVar(&c.OAuth.ClientID, "mcp-oauth-client-id", c.OAuth.ClientID, "OAuth client id")
	flag.StringVar(&c.OAuth.ClientSecret, "mcp-oauth-client-secret", c.OAuth.ClientSecret, "OAuth client secret")
	flag.Var(newCSVValue(c.OAuth.Scopes, &c.OAuth.Scopes), "mcp-oauth-scopes", "OAuth scopes")
	flag.StringVar(&c.OAuth.TokenFile, "mcp-oauth-token-file", c.OAuth.TokenFile, "path to OAuth token cache file")
	flag.BoolVar(&c.EnableLegacySSE, "mcp-enable-legacy-sse", c.EnableLegacySSE, "enable legacy SSE transport")
}

// LoadFile populates the config from a YAML file. Fields already set remain unless
// the file provides overrides. Environment variables and flags should be bound
// after loading to take precedence.
func (c *Config) LoadFile(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return err
	}
	mergeConfig(&fileCfg, c)
	*c = fileCfg
	return nil
}

func mergeConfig(dst *Config, src *Config) {
	if src.ConfigFile != "" {
		dst.ConfigFile = src.ConfigFile
	}
	if len(src.Order) != 0 {
		dst.Order = src.Order
	}
	if src.InitTimeout != 0 {
		dst.InitTimeout = src.InitTimeout
	}
	if src.ProtocolVersion != "" {
		dst.ProtocolVersion = src.ProtocolVersion
	}
	if src.MaxInFlight != 0 {
		dst.MaxInFlight = src.MaxInFlight
	}
	if src.Stdio.Command != "" {
		dst.Stdio.Command = src.Stdio.Command
	}
	if len(src.Stdio.Args) != 0 {
		dst.Stdio.Args = src.Stdio.Args
	}
	if len(src.Stdio.Env) != 0 {
		dst.Stdio.Env = src.Stdio.Env
	}
	if src.Stdio.WorkDir != "" {
		dst.Stdio.WorkDir = src.Stdio.WorkDir
	}
	if src.Stdio.AllowRelative {
		dst.Stdio.AllowRelative = true
	}
	if src.HTTP.URL != "" {
		dst.HTTP.URL = src.HTTP.URL
	}
	if src.HTTP.Timeout != 0 {
		dst.HTTP.Timeout = src.HTTP.Timeout
	}
	if src.HTTP.EnablePush {
		dst.HTTP.EnablePush = true
	}
	if src.HTTP.InsecureSkipVerify {
		dst.HTTP.InsecureSkipVerify = true
	}
	if src.OAuth.Enabled {
		dst.OAuth.Enabled = true
	}
	if src.OAuth.TokenURL != "" {
		dst.OAuth.TokenURL = src.OAuth.TokenURL
	}
	if src.OAuth.ClientID != "" {
		dst.OAuth.ClientID = src.OAuth.ClientID
	}
	if src.OAuth.ClientSecret != "" {
		dst.OAuth.ClientSecret = src.OAuth.ClientSecret
	}
	if len(src.OAuth.Scopes) != 0 {
		dst.OAuth.Scopes = src.OAuth.Scopes
	}
	if src.OAuth.TokenStore != nil {
		dst.OAuth.TokenStore = src.OAuth.TokenStore
	}
	if src.OAuth.TokenFile != "" {
		dst.OAuth.TokenFile = src.OAuth.TokenFile
	}
	if src.EnableLegacySSE {
		dst.EnableLegacySSE = true
	}
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

func parseDuration(v string) time.Duration {
	d, _ := time.ParseDuration(v)
	return d
}

func parseInt(v string) int {
	i, _ := strconv.Atoi(v)
	return i
}

// helper for flag CSV values
type csvValue struct {
	val []string
	dst *[]string
}

func newCSVValue(val []string, dst *[]string) *csvValue { return &csvValue{val: val, dst: dst} }

func (c *csvValue) String() string { return strings.Join(c.val, ",") }

func (c *csvValue) Set(v string) error {
	c.val = splitComma(v)
	*c.dst = c.val
	return nil
}

func getEnv(k, d string) string {
	if v := env(k); v != "" {
		return v
	}
	return d
}

var env = os.Getenv
