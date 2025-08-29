package agent

import (
    "context"
    "net"
    "net/http"
    "time"
)

// ServeUntilContext starts an HTTP server bound to addr and shuts it down when ctx is done.
// It returns the resolved listen address.
func ServeUntilContext(ctx context.Context, addr string, handler http.Handler) (string, error) {
    srv := &http.Server{Handler: handler}
    ln, err := net.Listen("tcp", addr)
    if err != nil {
        return "", err
    }
    actual := ln.Addr().String()
    go func() {
        <-ctx.Done()
        c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        _ = srv.Shutdown(c)
    }()
    go func() { _ = srv.Serve(ln) }()
    return actual, nil
}

