package ctrlsrv

import (
	"sort"
)

// ModelInfo represents an aggregated view of a model across workers.
type ModelInfo struct {
	ID      string
	Created int64
	Owners  []string
}

// AggregatedModels returns a list of models with ownership information.
func (r *Registry) AggregatedModels() []ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m := make(map[string]*ModelInfo)
	for _, w := range r.workers {
		w.mu.Lock()
		for id := range w.Models {
			info, ok := m[id]
			if !ok {
				info = &ModelInfo{ID: id, Created: r.modelFirstSeen[id]}
				m[id] = info
			}
			info.Owners = append(info.Owners, w.Name)
		}
		w.mu.Unlock()
	}
	var res []ModelInfo
	for _, info := range m {
		sort.Strings(info.Owners)
		res = append(res, *info)
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	return res
}

// AggregatedModel returns the aggregated info for a single model id.
func (r *Registry) AggregatedModel(id string) (ModelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var owners []string
	for _, w := range r.workers {
		w.mu.Lock()
		if w.Models[id] {
			owners = append(owners, w.Name)
		}
		w.mu.Unlock()
	}
	if len(owners) == 0 {
		return ModelInfo{}, false
	}
	sort.Strings(owners)
	return ModelInfo{ID: id, Created: r.modelFirstSeen[id], Owners: owners}, true
}
