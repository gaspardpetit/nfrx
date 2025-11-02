package auth

import (
	"net/http"
	"strings"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
)

type Mode string

const (
	ModeNone   Mode = "none"
	ModeSecret Mode = "secret"
)

type Strategy struct {
	Public []Mode
	Agent  []Mode
}

func BearerSecretMiddleware(secret string) spi.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				next.ServeHTTP(w, r)
				return
			}
			tok := ExtractBearer(r)
			if tok == "" || tok != secret {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// BearerOrRolesMiddleware authorizes when either a valid bearer token is provided
// or when any role in X-User-Roles matches one of the allowedRoles.
// If both secret and allowedRoles are empty, it allows all requests.
func BearerOrRolesMiddleware(secret string, allowedRoles []string) spi.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// No auth configured
			if secret == "" && len(allowedRoles) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			// Check bearer token first
			tok := ExtractBearer(r)
			if tok != "" && secret != "" && tok == secret {
				next.ServeHTTP(w, r)
				return
			}
			// Check roles header
			if hasAnyAllowedRole(r.Header.Get("X-User-Roles"), allowedRoles) {
				next.ServeHTTP(w, r)
				return
			}
			// If token required and mismatched, or roles not matched
			if secret != "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			// Secret not set but roles configured and not matched => unauthorized
			if len(allowedRoles) > 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			// Fallback allow
			next.ServeHTTP(w, r)
		})
	}
}

func ExtractBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func CheckSecret(token, expected string) bool { return expected == "" || token == expected }

func hasAnyAllowedRole(header string, allowed []string) bool {
	if header == "" || len(allowed) == 0 {
		return false
	}
	// Build a set for allowed roles for O(1) lookups
	allow := map[string]struct{}{}
	for _, r := range allowed {
		rr := strings.TrimSpace(r)
		if rr != "" {
			allow[rr] = struct{}{}
		}
	}
	items := strings.Split(header, ",")
	for _, it := range items {
		if _, ok := allow[strings.TrimSpace(it)]; ok {
			return true
		}
	}
	return false
}
