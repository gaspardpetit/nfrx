package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	const (
		v    = "v0.0.1-test"
		sha  = "abcdef"
		date = "2000-01-02T03:04:05Z"
	)
	ldflags := fmt.Sprintf("-X main.version=%s -X main.buildSHA=%s -X main.buildDate=%s", v, sha, date)
	cmd := exec.Command("go", "run", "-ldflags", ldflags, ".", "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := fmt.Sprintf("llamapool-mcp version=%s sha=%s date=%s", v, sha, date)
	if got != want {
		t.Fatalf("unexpected output: got %q want %q", got, want)
	}
}
