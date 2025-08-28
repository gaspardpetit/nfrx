package server

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"

    "github.com/gaspardpetit/nfrx/server/internal/plugin"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

// StateViewHTML returns the registered HTML fragment for a plugin's state view.
func StateViewHTML(state *serverstate.Registry) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        id := chi.URLParam(r, "id")
        if id == "" {
            // bad request: no id
            w.WriteHeader(http.StatusBadRequest)
            return
        }
        if state == nil {
            // no registry available
            w.WriteHeader(http.StatusNoContent)
            return
        }
        el, ok := state.Get(id)
        if !ok || el.HTML == nil {
            // plugin view not provided
            w.WriteHeader(http.StatusNoContent)
            return
        }
        html := el.HTML()
        if html == "" {
            // empty view
            w.WriteHeader(http.StatusNoContent)
            return
        }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte(html))
    }
}

// StateDescriptors returns the registered plugin descriptors as JSON.
func StateDescriptors() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        d := plugin.Descriptors()
        _ = json.NewEncoder(w).Encode(d)
    }
}
