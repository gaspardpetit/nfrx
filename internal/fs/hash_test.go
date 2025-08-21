package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashSHA256(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	sum, err := Hash(p, "sha256")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	const want = "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if sum != want {
		t.Fatalf("unexpected hash: got %s want %s", sum, want)
	}
}

func TestHashUnsupported(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Hash(p, "bad"); err == nil {
		t.Fatalf("expected error")
	}
}
