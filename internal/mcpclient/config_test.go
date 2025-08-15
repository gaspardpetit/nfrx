package mcpclient

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_LoadFileAndEnv(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cfg.yaml")
	data := []byte("order: [stdio]\ninitTimeout: 10s\nstdio:\n  command: cmd\n  workDir: /tmp\n")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	var cfg Config
	if err := cfg.LoadFile(file); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if cfg.Stdio.WorkDir != "/tmp" {
		t.Fatalf("workdir from yaml: %v", cfg.Stdio.WorkDir)
	}
	t.Setenv("MCP_TRANSPORT_ORDER", "http")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	old := flag.CommandLine
	flag.CommandLine = fs
	t.Cleanup(func() { flag.CommandLine = old })
	cfg.BindFlags()
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Order) != 1 || cfg.Order[0] != "http" {
		t.Fatalf("env override, got %v", cfg.Order)
	}
}
