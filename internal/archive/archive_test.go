package archive

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func createTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("A"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("B"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return dir
}

func listRelPaths(t *testing.T, root string) map[string]fs.FileInfo {
	m := map[string]fs.FileInfo{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		m[rel] = info
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return m
}

func TestZipUnzip(t *testing.T) {
	src := createTree(t)
	zipFile := filepath.Join(t.TempDir(), "test.zip")
	if err := Zip(src, zipFile, nil, nil); err != nil {
		t.Fatalf("zip: %v", err)
	}
	dest := t.TempDir()
	if err := Unzip(zipFile, dest, nil, nil); err != nil {
		t.Fatalf("unzip: %v", err)
	}
	want := listRelPaths(t, src)
	got := listRelPaths(t, dest)
	if len(want) != len(got) {
		t.Fatalf("file count mismatch: want %d got %d", len(want), len(got))
	}
	for rel, info := range want {
		g, ok := got[rel]
		if !ok {
			t.Fatalf("missing %s", rel)
		}
		if info.IsDir() != g.IsDir() {
			t.Fatalf("dir mismatch for %s", rel)
		}
	}
}

func TestTarUntar(t *testing.T) {
	src := createTree(t)
	tarFile := filepath.Join(t.TempDir(), "test.tar")
	if err := Tar(src, tarFile, nil, nil); err != nil {
		t.Fatalf("tar: %v", err)
	}
	dest := t.TempDir()
	if err := Untar(tarFile, dest, nil, nil); err != nil {
		t.Fatalf("untar: %v", err)
	}
	want := listRelPaths(t, src)
	got := listRelPaths(t, dest)
	if len(want) != len(got) {
		t.Fatalf("file count mismatch: want %d got %d", len(want), len(got))
	}
	for rel, info := range want {
		g, ok := got[rel]
		if !ok {
			t.Fatalf("missing %s", rel)
		}
		if info.IsDir() != g.IsDir() {
			t.Fatalf("dir mismatch for %s", rel)
		}
	}
}
