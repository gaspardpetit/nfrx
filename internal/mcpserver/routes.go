package mcpserver

import "github.com/go-chi/chi/v5"

// RegisterRoutes registers HTTP routes; MCP uses relay endpoints only.
func (p *Plugin) RegisterRoutes(r chi.Router) {}

// RegisterRelayEndpoints wires MCP relay HTTP/WS endpoints.
func (p *Plugin) RegisterRelayEndpoints(r chi.Router) {
	r.Handle("/api/mcp/connect", p.broker.WSHandler(p.cfg.ClientKey))
	r.Post("/api/mcp/id/{id}", p.broker.HTTPHandler())
}
