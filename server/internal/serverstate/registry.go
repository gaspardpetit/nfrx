package serverstate

import "sync"

// Element represents a plugin-contributed state element.
type Element struct {
	ID   string
	Data func() interface{}
	HTML func() string
}

// Registry collects state elements provided by plugins.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]Element
}

// NewRegistry returns a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]Element)}
}

// Add registers a new state element.
func (r *Registry) Add(e Element) {
	r.mu.Lock()
	r.entries[e.ID] = e
	r.mu.Unlock()
}

// Elements returns all registered state elements.
func (r *Registry) Elements() []Element {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]Element, 0, len(r.entries))
	for _, e := range r.entries {
		res = append(res, e)
	}
	return res
}

// Get returns the element for a given ID, if present.
func (r *Registry) Get(id string) (Element, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	return e, ok
}
