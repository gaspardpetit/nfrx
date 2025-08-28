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

    // Agent-common metrics (aggregated across all extensions)
    agentJobsInflight = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "nfrx_agent_jobs_inflight",
            Help: "Number of in-flight jobs across all agents",
        },
    )

    agentJobsTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "nfrx_agent_jobs_total",
            Help: "Total number of jobs processed by agents",
        },
    )

    agentJobsFailedTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "nfrx_agent_jobs_failed_total",
            Help: "Total number of failed jobs reported by agents",
        },
    )
)

// Register registers server-specific and agent-common metrics.
func Register(r spi.MetricsRegistry) {
    r.MustRegister(buildInfo, agentJobsInflight, agentJobsTotal, agentJobsFailedTotal)
}

// SetServerBuildInfo sets the build info metric for the server.
func SetServerBuildInfo(version, sha, date string) {
    buildInfo.WithLabelValues(date, sha, version).Set(1)
}

// AgentJobStart increments in-flight job gauge.
func AgentJobStart() { agentJobsInflight.Inc() }

// AgentJobEnd decrements in-flight and increments totals; marks failures when !success.
func AgentJobEnd(success bool) {
    agentJobsInflight.Dec()
    agentJobsTotal.Inc()
    if !success {
        agentJobsFailedTotal.Inc()
    }
}
