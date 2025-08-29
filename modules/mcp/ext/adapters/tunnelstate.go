package adapters

import (
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	"github.com/gaspardpetit/nfrx/sdk/base/tunnel"
)

// TunnelStateToMCP converts a generic tunnel.State to spi.MCPState for UI/state rendering.
func TunnelStateToMCP(st tunnel.State) spi.MCPState {
	out := spi.MCPState{}
	for _, c := range st.Clients {
		funcs := make(map[string]int, len(c.Methods))
		for k, v := range c.Methods {
			funcs[k] = v
		}
		out.Clients = append(out.Clients, spi.MCPClientSnapshot{ID: c.ID, Name: c.Name, Status: c.Status, Inflight: c.Inflight, Functions: funcs})
	}
	for _, s := range st.Sessions {
		out.Sessions = append(out.Sessions, spi.MCPSessionSnapshot{ID: s.ID, ClientID: s.ClientID, Method: s.Method, StartedAt: s.StartedAt, DurationMs: s.DurationMs})
	}
	return out
}
