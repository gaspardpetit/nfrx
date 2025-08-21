package serverstate

import (
	"sync/atomic"
)

var state atomic.Value
var draining atomic.Bool

func init() {
	state.Store("not_ready")
}

// SetState sets the server state string.
func SetState(s string) {
	state.Store(s)
}

// GetState returns the current server state.
func GetState() string {
	if v, ok := state.Load().(string); ok {
		return v
	}
	return "unknown"
}

// StartDrain marks the server as draining.
func StartDrain() {
	draining.Store(true)
	SetState("draining")
}

// IsDraining reports whether the server is draining.
func IsDraining() bool {
	return draining.Load()
}
