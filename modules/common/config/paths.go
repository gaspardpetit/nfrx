package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultConfigPath returns the default config file path for the given component
// name (e.g. "worker.yaml", "server.yaml").
func DefaultConfigPath(name string) string {
	home, _ := os.UserHomeDir()
	programData := os.Getenv("ProgramData")
	return ResolveConfigPath(runtime.GOOS, home, programData, name)
}

// ResolveConfigPath constructs a config file path for the given OS and base
// directories. It is mainly used in tests.
func ResolveConfigPath(goos, home, programData, name string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "nfrx", name)
	case "windows":
		if programData == "" {
			programData = "C:/ProgramData"
		}
		programData = strings.TrimRight(programData, "\\/")
		return filepath.Join(programData, "nfrx", name)
	default:
		return filepath.Join("/etc", "nfrx", name)
	}
}
