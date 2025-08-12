package api

import (
	"encoding/json"
	"net/http"

	"github.com/you/llamapool/internal/ctrl"
)

// TagsHandler handles GET /api/tags.
func TagsHandler(reg *ctrl.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		models := reg.Models()
		type m struct {
			Name string `json:"name"`
		}
		var resp struct {
			Models []m `json:"models"`
		}
		for _, mod := range models {
			resp.Models = append(resp.Models, m{Name: mod})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
