package server

import (
	_ "embed"
	"net/http"
)

//go:embed status.html
var statusHTML string

// StatusHandler serves the embedded status page.
func StatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(statusHTML))
	}
}
