package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/you/llamapool/internal/ctrl"
)

// ListModelsHandler handles GET /v1/models.
func ListModelsHandler(reg *ctrl.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		models := reg.AggregatedModels()
		type item struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		}
		var resp struct {
			Object string `json:"object"`
			Data   []item `json:"data"`
		}
		resp.Object = "list"
		for _, m := range models {
			resp.Data = append(resp.Data, item{ID: m.ID, Object: "model", Created: m.Created, OwnedBy: strings.Join(m.Owners, ",")})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// GetModelHandler handles GET /v1/models/{id}.
func GetModelHandler(reg *ctrl.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		m, ok := reg.AggregatedModel(id)
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "model_not_found"})
			return
		}
		resp := struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		}{ID: m.ID, Object: "model", Created: m.Created, OwnedBy: strings.Join(m.Owners, ",")}
		json.NewEncoder(w).Encode(resp)
	}
}
