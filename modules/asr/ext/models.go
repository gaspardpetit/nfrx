package asr

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gaspardpetit/nfrx/core/logx"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// modelTracker tracks first-seen timestamps for models.
type modelTracker struct {
	mu        sync.Mutex
	firstSeen map[string]int64
}

// listModelsHandler aggregates models across workers.
func listModelsHandler(reg *baseworker.Registry, mt *modelTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws := reg.Snapshot()
		ownersMap := make(map[string][]string)

		mt.mu.Lock()
		if mt.firstSeen == nil {
			mt.firstSeen = make(map[string]int64)
		}
		now := time.Now().Unix()
		for _, w := range ws {
			name := w.NameValue()
			for _, id := range w.LabelKeys() {
				ownersMap[id] = append(ownersMap[id], name)
				if _, ok := mt.firstSeen[id]; !ok {
					mt.firstSeen[id] = now
				}
			}
		}
		type item struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		}
		var data []item
		for id, owners := range ownersMap {
			sort.Strings(owners)
			data = append(data, item{ID: id, Object: "model", Created: mt.firstSeen[id], OwnedBy: strings.Join(owners, ",")})
		}
		sort.Slice(data, func(i, j int) bool { return data[i].ID < data[j].ID })
		mt.mu.Unlock()

		resp := struct {
			Object string `json:"object"`
			Data   []item `json:"data"`
		}{Object: "list", Data: data}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logx.Log.Error().Err(err).Msg("encode models list")
		}
	}
}

// getModelHandler returns details for a specific model.
func getModelHandler(reg *baseworker.Registry, mt *modelTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ws := reg.Snapshot()

		var owners []string
		mt.mu.Lock()
		if mt.firstSeen == nil {
			mt.firstSeen = make(map[string]int64)
		}
		now := time.Now().Unix()
		for _, w := range ws {
			if w.HasLabel(id) {
				owners = append(owners, w.NameValue())
			}
		}
		if len(owners) == 0 {
			mt.mu.Unlock()
			logx.Log.Warn().Str("model", id).Msg("model not found")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": "model_not_found"}); err != nil {
				logx.Log.Error().Err(err).Msg("encode model not found")
			}
			return
		}
		if _, ok := mt.firstSeen[id]; !ok {
			mt.firstSeen[id] = now
		}
		sort.Strings(owners)
		created := mt.firstSeen[id]
		mt.mu.Unlock()

		resp := struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		}{ID: id, Object: "model", Created: created, OwnedBy: strings.Join(owners, ",")}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logx.Log.Error().Err(err).Msg("encode model detail")
		}
	}
}
