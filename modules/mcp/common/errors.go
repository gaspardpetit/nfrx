package common

// Canonical MCP error codes used in adapters and agent
const (
    ErrUnauthorized       = "MCP_UNAUTHORIZED"
    ErrProviderUnavailable = "MCP_PROVIDER_UNAVAILABLE"
    ErrTimeout            = "MCP_TIMEOUT"
    ErrLimitExceeded      = "MCP_LIMIT_EXCEEDED"
    ErrUpstreamError      = "MCP_UPSTREAM_ERROR"
    ErrSchema             = "MCP_SCHEMA_ERROR"
)
