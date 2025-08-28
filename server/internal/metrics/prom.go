package metrics

import (
    "github.com/prometheus/client_golang/prometheus"

    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

var (
    buildInfo = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name:        "nfrx_server_build_info",
            Help:        "Build information for the nfrx server",
            ConstLabels: prometheus.Labels{"component": "server"},
        },
        []string{"date", "sha", "version"},
    )
)

// Register registers server-specific metrics with the provided registerer.
func Register(r spi.MetricsRegistry) { r.MustRegister(buildInfo) }

// SetServerBuildInfo sets the build info metric for the server.
func SetServerBuildInfo(version, sha, date string) {
    buildInfo.WithLabelValues(date, sha, version).Set(1)
}
