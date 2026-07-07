package storage

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveOpenDeletePreservesFilename(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	rel, size, err := store.Save(42, "Quote V2.pdf", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if size != 5 {
		t.Errorf("expected size 5, got %d", size)
	}
	// Original filename preserved as the basename, under the PR id folder.
	if filepath.Base(rel) != "Quote V2.pdf" {
		t.Errorf("filename not preserved: %q", rel)
	}
	if !strings.HasPrefix(rel, filepath.Join("purchase-requests", "42")) {
		t.Errorf("unexpected layout: %q", rel)
	}

	f, err := store.Open(rel)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	data, _ := io.ReadAll(f)
	f.Close()
	if string(data) != "hello" {
		t.Errorf("content mismatch: %q", data)
	}

	if err := store.Delete(rel); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.Open(rel); err == nil {
		t.Error("expected error opening deleted file")
	}
}

func TestResolveRejectsTraversal(t *testing.T) {
	store, _ := NewLocalStore(t.TempDir())
	if _, err := store.Open("../../etc/passwd"); err == nil {
		t.Error("expected path traversal to be rejected")
	}
}
