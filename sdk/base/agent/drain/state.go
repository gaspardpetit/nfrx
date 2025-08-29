package drain

import (
    "sync"
    "sync/atomic"
)

var (
    draining atomic.Bool
    mu       sync.Mutex
    onCheck  func()
)

// Start sets draining state to true and triggers callback.
func Start() { draining.Store(true); trigger() }
// Stop sets draining state to false and triggers callback.
func Stop()  { draining.Store(false); trigger() }
// IsDraining reports whether draining is active.
func IsDraining() bool { return draining.Load() }

// OnCheck registers a callback invoked when state changes.
func OnCheck(fn func()) { mu.Lock(); defer mu.Unlock(); onCheck = fn }

func trigger() { mu.Lock(); fn := onCheck; mu.Unlock(); if fn != nil { fn() } }

