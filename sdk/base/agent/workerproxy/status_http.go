package workerproxy

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	dr "github.com/gaspardpetit/nfrx/sdk/base/agent/drain"
)

// StartStatusServer exposes the agent status and drain controls.
// The token file is created next to the provided configFile using TokenBasename.
func StartStatusServer(ctx context.Context, addr, tokenBasename, configFile string, drainTimeout time.Duration, shutdown func()) (string, error) {
	if strings.TrimSpace(tokenBasename) == "" {
		tokenBasename = "agent"
	}
	tokenPath := filepath.Join(filepath.Dir(defaultConfigPath(configFile)), tokenBasename+".token")
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
		return "agent.yaml"
	}
	return p
}
