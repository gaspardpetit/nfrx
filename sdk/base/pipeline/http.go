package pipeline

import (
    "net/http"

    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

type HTTPFilter interface {
    Wrap(http.Handler) http.Handler
}

type MiddlewareFilter struct{ MW spi.Middleware }

func (f MiddlewareFilter) Wrap(h http.Handler) http.Handler {
    if f.MW == nil { return h }
    return f.MW(h)
}

func Chain(h http.Handler, filters ...HTTPFilter) http.Handler {
    wrapped := h
    for i := len(filters) - 1; i >= 0; i-- {
        wrapped = filters[i].Wrap(wrapped)
    }
    return wrapped
}

