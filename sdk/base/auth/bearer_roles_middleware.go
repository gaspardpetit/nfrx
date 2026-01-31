package auth

import (
	"net/http"
	"strings"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
)

// BearerAnyOrRolesMiddleware authorizes when the bearer token matches any secret
// or when any role in X-User-Roles matches one of the allowedRoles.
// If both secrets and allowedRoles are empty, it allows all requests.
func BearerAnyOrRolesMiddleware(secrets []string, allowedRoles []string) spi.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(secrets) == 0 && len(allowedRoles) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			tok := ExtractBearer(r)
			if tok != "" && matchesAnySecret(tok, secrets) {
				next.ServeHTTP(w, r)
				return
			}
			if hasAnyAllowedRole(r.Header.Get("X-User-Roles"), allowedRoles) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		})
	}
}

func matchesAnySecret(token string, secrets []string) bool {
	if token == "" || len(secrets) == 0 {
		return false
	}
	for _, s := range secrets {
		ss := strings.TrimSpace(s)
		if ss != "" && token == ss {
			return true
		}
	}
	return false
}
