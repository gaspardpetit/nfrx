package mcpclient

import (
	"encoding/json"
	"os"

	"github.com/mark3labs/mcp-go/client/transport"
)

// FileTokenStore persists OAuth tokens to disk with 0600 permissions.
// It is a minimal implementation and does not encrypt the contents.
type FileTokenStore struct {
	path string
}

// NewFileTokenStore returns a TokenStore backed by the given file path.
func NewFileTokenStore(path string) *FileTokenStore { return &FileTokenStore{path: path} }

// GetToken reads the token from disk.
func (s *FileTokenStore) GetToken() (*transport.Token, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var tok transport.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

// SaveToken writes the token to disk with 0600 permissions.
func (s *FileTokenStore) SaveToken(tok *transport.Token) error {
	data, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
