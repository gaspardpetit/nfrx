package metrics

import (
    "testing"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/testutil"

    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

func TestServerBuildInfo(t *testing.T) {
    reg := testRegistry{prometheus.NewRegistry()}
    Register(reg)
    SetServerBuildInfo("1.0.0", "abc", "2024-01-01")
    if v := testutil.ToFloat64(buildInfo.WithLabelValues("2024-01-01", "abc", "1.0.0")); v != 1 {
        t.Fatalf("build info: %v", v)
    }
}

type testRegistry struct{ *prometheus.Registry }

func (r testRegistry) MustRegister(cs ...spi.Collector) {
    collectors := make([]prometheus.Collector, 0, len(cs))
    for _, c := range cs {
        collectors = append(collectors, c.(prometheus.Collector))
    }
    r.Registry.MustRegister(collectors...)
}
