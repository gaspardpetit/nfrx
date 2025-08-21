package fs

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

// Hash computes the checksum of the file at path using the given algorithm.
// Supported algorithms: sha256, sha1, md5.
func Hash(path, algo string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	var h hash.Hash
	switch strings.ToLower(algo) {
	case "sha256", "sha-256":
		h = sha256.New()
	case "sha1", "sha-1":
		h = sha1.New()
	case "md5":
		h = md5.New()
	default:
		return "", fmt.Errorf("unsupported hash algorithm %q", algo)
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
