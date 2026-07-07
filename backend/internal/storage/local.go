// Package storage handles purchase-request document files on the local
// filesystem. Files are stored under a configurable root, with the original
// filename preserved inside a per-document UUID folder to avoid collisions:
//
//	<root>/purchase-requests/<prID>/<docUUID>/<originalFilename>
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// LocalStore stores files under an absolute root directory.
type LocalStore struct {
	root string
}

// NewLocalStore resolves root to an absolute path and ensures it exists.
func NewLocalStore(root string) (*LocalStore, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	return &LocalStore{root: abs}, nil
}

// Root returns the absolute storage root.
func (s *LocalStore) Root() string { return s.root }

// Save streams r into a new file for the given purchase request, preserving the
// original filename. It returns the path relative to the root (stored in the DB)
// and the number of bytes written.
func (s *LocalStore) Save(prID int64, originalName string, r io.Reader) (relPath string, size int64, err error) {
	clean := sanitizeName(originalName)
	docUUID := uuid.NewString()
	relDir := filepath.Join("purchase-requests", fmt.Sprintf("%d", prID), docUUID)
	absDir := filepath.Join(s.root, relDir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create document dir: %w", err)
	}

	relPath = filepath.Join(relDir, clean)
	absPath := filepath.Join(s.root, relPath)
	f, err := os.Create(absPath)
	if err != nil {
		return "", 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	size, err = io.Copy(f, r)
	if err != nil {
		_ = os.Remove(absPath)
		return "", 0, fmt.Errorf("write file: %w", err)
	}
	return relPath, size, nil
}

// Open opens a stored file for reading. relPath must stay within the root.
func (s *LocalStore) Open(relPath string) (io.ReadCloser, error) {
	abs, err := s.resolve(relPath)
	if err != nil {
		return nil, err
	}
	return os.Open(abs)
}

// Delete removes a stored file (and its now-empty UUID dir). relPath must stay
// within the root.
func (s *LocalStore) Delete(relPath string) error {
	abs, err := s.resolve(relPath)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Best-effort cleanup of the per-document UUID directory.
	_ = os.Remove(filepath.Dir(abs))
	return nil
}

// resolve joins relPath onto the root and rejects any path that escapes it.
func (s *LocalStore) resolve(relPath string) (string, error) {
	abs := filepath.Join(s.root, filepath.Clean("/"+relPath))
	if abs != s.root && !strings.HasPrefix(abs, s.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path %q", relPath)
	}
	return abs, nil
}

// sanitizeName strips any directory components from an uploaded filename,
// preserving just the base name. Falls back to a uuid if empty.
func sanitizeName(name string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." || base == "/" {
		return uuid.NewString()
	}
	return base
}
