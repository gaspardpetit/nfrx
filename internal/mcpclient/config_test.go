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

func TestConfig_SecurityFlags(t *testing.T) {
	var cfg Config
	t.Setenv("MCP_STDIO_ALLOW_RELATIVE", "true")
	t.Setenv("MCP_HTTP_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("MCP_OAUTH_TOKEN_FILE", "/tmp/token.json")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	old := flag.CommandLine
	flag.CommandLine = fs
	t.Cleanup(func() { flag.CommandLine = old })
	cfg.BindFlags()
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Stdio.AllowRelative {
		t.Fatalf("allow relative not set")
	}
	if !cfg.HTTP.InsecureSkipVerify {
		t.Fatalf("insecure skip verify not set")
	}
	if cfg.OAuth.TokenFile != "/tmp/token.json" {
		t.Fatalf("token file mismatch: %s", cfg.OAuth.TokenFile)
	}
}
