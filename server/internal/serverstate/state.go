package serverstate

import "sync/atomic"

// State holds the server status and draining flag. All fields are updated
// together so callers always observe a consistent snapshot.
type State struct {
	Status   string
	Draining bool
}

// Store defines how the server state is persisted. Implementations may store
// state in memory, on disk, or in an external service such as Redis.
type Store interface {
	Load() State
	Store(State)
}

// active is the currently configured Store. It defaults to an in-memory
// implementation but can be swapped for other strategies.
var active Store = NewMemoryStore()

// UseStore replaces the active Store. It is safe for concurrent use.
func UseStore(s Store) {
	if s != nil {
		active = s
	}
}

// memoryStore implements Store using an atomic.Value. It is the default
// strategy and is safe for concurrent use within a single process.
type memoryStore struct {
	v atomic.Value
}

// NewMemoryStore returns a memory-backed Store initialized to "not_ready".
func NewMemoryStore() *memoryStore {
	ms := &memoryStore{}
	ms.v.Store(State{Status: "not_ready"})
	return ms
}

func (m *memoryStore) Load() State {
	if st, ok := m.v.Load().(State); ok {
		return st
	}
	return State{Status: "unknown"}
}

func (m *memoryStore) Store(s State) {
	m.v.Store(s)
}

// SetState updates the server status string.
func SetState(status string) {
	st := active.Load()
	st.Status = status
	active.Store(st)
}

// GetState returns the current server status.
func GetState() string {
	return active.Load().Status
}

// StartDrain marks the server as draining.
func StartDrain() {
	st := active.Load()
	st.Draining = true
	st.Status = "draining"
	active.Store(st)
}

// IsDraining reports whether the server is draining.
func IsDraining() bool {
	return active.Load().Draining
}
