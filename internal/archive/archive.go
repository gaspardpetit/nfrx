package archive

import (
	"archive/tar"
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Zip creates a zip archive at dest from the contents of src.
// Files are included/excluded based on the provided glob patterns.
func Zip(src, dest string, include, exclude []string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if !shouldInclude(rel+"/", include, exclude) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldInclude(rel, include, exclude) {
			return nil
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		rf, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = rf.Close() }()
		_, err = io.Copy(w, rf)
		return err
	})
}

// Unzip extracts the zip archive at src into dest applying include/exclude globs.
func Unzip(src, dest string, include, exclude []string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	for _, f := range r.File {
		name := f.Name
		if f.FileInfo().IsDir() {
			name = strings.TrimSuffix(name, "/") + "/"
		}
		if !shouldInclude(name, include, exclude) {
			continue
		}
		path := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = rc.Close()
			_ = out.Close()
			return err
		}
		_ = rc.Close()
		_ = out.Close()
	}
	return nil
}

// Tar creates a tar archive at dest from the contents of src.
func Tar(src, dest string, include, exclude []string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	tw := tar.NewWriter(f)
	defer func() { _ = tw.Close() }()
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if !shouldInclude(rel+"/", include, exclude) {
				return filepath.SkipDir
			}
		}
		if info.IsDir() && rel == "." {
			return nil
		}
		if info.IsDir() {
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = rel + "/"
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			return nil
		}
		if !shouldInclude(rel, include, exclude) {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		rf, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = rf.Close() }()
		_, err = io.Copy(tw, rf)
		return err
	})
}

// Untar extracts the tar archive at src into dest applying include/exclude globs.
func Untar(src, dest string, include, exclude []string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(hdr.Name)
		if hdr.FileInfo().IsDir() {
			name = strings.TrimSuffix(name, "/") + "/"
		}
		if !shouldInclude(name, include, exclude) {
			continue
		}
		path := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			_ = out.Close()
		}
	}
	return nil
}

func shouldInclude(rel string, include, exclude []string) bool {
	for _, pattern := range exclude {
		if ok, _ := doublestar.PathMatch(pattern, rel); ok {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, pattern := range include {
		if ok, _ := doublestar.PathMatch(pattern, rel); ok {
			return true
		}
	}
	return false
}
