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

func ExtractBearer(r *http.Request) string {
    auth := r.Header.Get("Authorization")
    if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
        return strings.TrimSpace(auth[7:])
    }
    return ""
}

func CheckSecret(token, expected string) bool { return expected == "" || token == expected }

