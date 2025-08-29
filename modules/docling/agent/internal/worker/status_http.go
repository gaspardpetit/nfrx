package worker

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	dr "github.com/gaspardpetit/nfrx/sdk/base/agent/drain"
)

func StartStatusServer(ctx context.Context, addr, configFile string, drainTimeout time.Duration, shutdown func()) (string, error) {
	tokenPath := filepath.Join(filepath.Dir(defaultConfigPath(configFile)), "docling.token")
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
		return "docling.yaml"
	}
	return p
}
