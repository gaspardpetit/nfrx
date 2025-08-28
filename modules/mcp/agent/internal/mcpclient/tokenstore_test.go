package mcpclient

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/client/transport"
)

func TestFileTokenStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tok.json")
	store := NewFileTokenStore(path)
	tok := &transport.Token{AccessToken: "abc", TokenType: "Bearer"}
	if err := store.SaveToken(tok); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v", info.Mode())
	}
	got, err := store.GetToken()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.AccessToken != tok.AccessToken {
		t.Fatalf("token mismatch: got %s", got.AccessToken)
	}
}
