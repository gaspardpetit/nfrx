package config

import (
	"strings"
	"testing"
)

func TestResolveWorkerPaths(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		home        string
		programData string
		wantConfig  string
		wantLogDir  string
	}{
		{
			name:       "linux",
			goos:       "linux",
			home:       "/home/user",
			wantConfig: "/etc/nfrx/worker.yaml",
			wantLogDir: "",
		},
		{
			name:       "darwin",
			goos:       "darwin",
			home:       "/Users/test",
			wantConfig: "/Users/test/Library/Application Support/nfrx/worker.yaml",
			wantLogDir: "/Users/test/Library/Logs/nfrx",
		},
		{
			name:        "windows",
			goos:        "windows",
			programData: "C:\\ProgramData",
			wantConfig:  "C:/ProgramData/nfrx/worker.yaml",
			wantLogDir:  "C:/ProgramData/nfrx/Logs",
		},
		{
			name:       "windows default ProgramData",
			goos:       "windows",
			wantConfig: "C:/ProgramData/nfrx/worker.yaml",
			wantLogDir: "C:/ProgramData/nfrx/Logs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, log := resolveWorkerPaths(tt.goos, tt.home, tt.programData)
			cfg = strings.ReplaceAll(cfg, "\\", "/")
			log = strings.ReplaceAll(log, "\\", "/")
			if cfg != tt.wantConfig {
				t.Errorf("config path: got %q want %q", cfg, tt.wantConfig)
			}
			if log != tt.wantLogDir {
				t.Errorf("log dir: got %q want %q", log, tt.wantLogDir)
			}
		})
	}
}
