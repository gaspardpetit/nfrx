package agent

import (
    "context"
    "net/http"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartMetricsServer exposes a Prometheus handler backed by the provided registry.
// The registry may be nil to use the default global registry.
func StartMetricsServer(ctx context.Context, addr string, reg prometheus.Gatherer) (string, error) {
    mux := http.NewServeMux()
    if reg == nil {
        mux.Handle("/metrics", promhttp.Handler())
    } else {
        mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
    }
    return ServeUntilContext(ctx, addr, mux)
}

