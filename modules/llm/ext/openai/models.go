package openai

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/modules/common/logx"
)

// ListModelsHandler handles GET /api/v1/models.
func ListModelsHandler(reg *ctrlsrv.Registry) http.HandlerFunc {
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
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logx.Log.Error().Err(err).Msg("encode models list")
		}
	}
}

// GetModelHandler handles GET /api/v1/models/{id}.
func GetModelHandler(reg *ctrlsrv.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		m, ok := reg.AggregatedModel(id)
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			logx.Log.Warn().Str("model", id).Msg("model not found")
			w.WriteHeader(http.StatusNotFound)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": "model_not_found"}); err != nil {
				logx.Log.Error().Err(err).Msg("encode model not found")
			}
			return
		}
		resp := struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		}{ID: m.ID, Object: "model", Created: m.Created, OwnedBy: strings.Join(m.Owners, ",")}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logx.Log.Error().Err(err).Msg("encode model detail")
		}
	}
}
