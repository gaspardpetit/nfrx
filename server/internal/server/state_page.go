package server

import (
	_ "embed"
	"net/http"
)

//go:embed state.html
var stateHTML string

// StateHandler serves the embedded state page.
func StatePageHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        _, _ = w.Write([]byte(stateHTML))
    }
}
