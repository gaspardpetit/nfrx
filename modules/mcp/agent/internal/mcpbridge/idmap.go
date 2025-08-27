package mcpbridge

import (
	"encoding/json"
	"strconv"
	"sync"
)

// IDMapper manages correlation IDs for JSON-RPC requests.
type IDMapper struct {
	mu    sync.Mutex
	next  uint64
	store map[string]json.RawMessage
}

// NewIDMapper constructs a new IDMapper.
func NewIDMapper() *IDMapper {
	return &IDMapper{store: make(map[string]json.RawMessage)}
}

// Alloc assigns a new correlation ID for the given JSON-RPC id.
// It stores the original id for later lookup.
func (m *IDMapper) Alloc(jsonID json.RawMessage) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := strconv.FormatUint(m.next, 10)
	m.next++
	m.store[id] = jsonID
	return id
}

// Resolve returns the original JSON-RPC id for the correlation id.
// If found, the mapping is removed.
func (m *IDMapper) Resolve(corrID string) (json.RawMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	jsonID, ok := m.store[corrID]
	if ok {
		delete(m.store, corrID)
	}
	return jsonID, ok
}
