package drain

import "sync/atomic"

var draining atomic.Bool

// Start marks the process as draining.
func Start() { draining.Store(true) }

// Stop clears the draining flag.
func Stop() { draining.Store(false) }

// IsDraining reports whether draining is in progress.
func IsDraining() bool { return draining.Load() }
