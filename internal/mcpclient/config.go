package mcpclient

import (
	"flag"
	"os"
	"strings"
	"time"
)

// Config controls how the MCP client connects to third-party servers.
type Config struct {
	// Order defines the ordered list of transports to attempt.
	// Valid entries: "stdio", "http", "oauth", "legacy-sse".
	Order []string

	// InitTimeout caps the time allowed for each transport to start and initialize.
	InitTimeout time.Duration

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
	Command string
	Args    []string
	Env     []string
}

// HTTPConfig describes a remote HTTP MCP server.
type HTTPConfig struct {
	URL string
}

// OAuthConfig contains optional OAuth parameters.
type OAuthConfig struct {
	Enabled      bool
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// BindFlags populates the config using environment variables and binds CLI flags.
func (c *Config) BindFlags() {
	order := getEnv("MCP_TRANSPORT_ORDER", "stdio,http,oauth")
	c.Order = splitComma(order)
	c.InitTimeout = parseDuration(getEnv("MCP_INIT_TIMEOUT", "5s"))

	c.Stdio.Command = getEnv("MCP_STDIO_COMMAND", "")
	c.Stdio.Args = splitComma(getEnv("MCP_STDIO_ARGS", ""))
	c.Stdio.Env = splitComma(getEnv("MCP_STDIO_ENV", ""))

	c.HTTP.URL = getEnv("MCP_HTTP_URL", "")

	c.OAuth.Enabled = getEnv("MCP_OAUTH_ENABLED", "false") == "true"
	c.OAuth.TokenURL = getEnv("MCP_OAUTH_TOKEN_URL", "")
	c.OAuth.ClientID = getEnv("MCP_OAUTH_CLIENT_ID", "")
	c.OAuth.ClientSecret = getEnv("MCP_OAUTH_CLIENT_SECRET", "")
	c.OAuth.Scopes = splitComma(getEnv("MCP_OAUTH_SCOPES", ""))

	c.EnableLegacySSE = getEnv("MCP_ENABLE_LEGACY_SSE", "false") == "true"

	flag.Func("mcp-transport-order", "comma separated transport order", func(v string) error { c.Order = splitComma(v); return nil })
	flag.DurationVar(&c.InitTimeout, "mcp-init-timeout", c.InitTimeout, "timeout for transport startup and initialization")
	flag.StringVar(&c.Stdio.Command, "mcp-stdio-command", c.Stdio.Command, "command for stdio transport")
	flag.Var(newCSVValue(c.Stdio.Args, &c.Stdio.Args), "mcp-stdio-args", "stdio command arguments")
	flag.Var(newCSVValue(c.Stdio.Env, &c.Stdio.Env), "mcp-stdio-env", "stdio environment variables")
	flag.StringVar(&c.HTTP.URL, "mcp-http-url", c.HTTP.URL, "HTTP MCP server base URL")
	flag.BoolVar(&c.OAuth.Enabled, "mcp-oauth-enabled", c.OAuth.Enabled, "enable OAuth for HTTP transport")
	flag.StringVar(&c.OAuth.TokenURL, "mcp-oauth-token-url", c.OAuth.TokenURL, "OAuth token endpoint")
	flag.StringVar(&c.OAuth.ClientID, "mcp-oauth-client-id", c.OAuth.ClientID, "OAuth client id")
	flag.StringVar(&c.OAuth.ClientSecret, "mcp-oauth-client-secret", c.OAuth.ClientSecret, "OAuth client secret")
	flag.Var(newCSVValue(c.OAuth.Scopes, &c.OAuth.Scopes), "mcp-oauth-scopes", "OAuth scopes")
	flag.BoolVar(&c.EnableLegacySSE, "mcp-enable-legacy-sse", c.EnableLegacySSE, "enable legacy SSE transport")
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
