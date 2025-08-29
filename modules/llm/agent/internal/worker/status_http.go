package worker

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	dr "github.com/gaspardpetit/nfrx/sdk/base/agent/drain"
)

// StartStatusServer starts an HTTP server exposing status, version, and control endpoints.
// The token for control endpoints is stored alongside the config file.
// It returns the address it is listening on.
func StartStatusServer(ctx context.Context, addr, configFile string, drainTimeout time.Duration, shutdown func()) (string, error) {
	tokenPath := filepath.Join(filepath.Dir(defaultConfigPath(configFile)), "worker.token")
	// Wrap shutdown to also update local state
	wrappedShutdown := func() {
		SetState("terminating")
		if shutdown != nil {
			shutdown()
		}
	}
	return dr.StartControlServer(ctx, addr, tokenPath, drainTimeout, func() any { return GetState() }, func() any { return GetVersionInfo() }, wrappedShutdown)
}

func defaultConfigPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return "worker.yaml"
	}
	return p
}
