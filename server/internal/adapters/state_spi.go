package adapters

import (
	"github.com/gaspardpetit/nfrx-sdk/spi"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

type StateRegistry struct{ *serverstate.Registry }

func NewStateRegistry(r *serverstate.Registry) StateRegistry { return StateRegistry{r} }

func (r StateRegistry) Add(el spi.StateElement) {
	r.Registry.Add(serverstate.Element{ID: el.ID, Data: el.Data})
}

type ServerState struct{}

func (ServerState) IsDraining() bool { return serverstate.IsDraining() }
