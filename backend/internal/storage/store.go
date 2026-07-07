package storage

import "io"

// Store abstracts document blob storage. Implemented by LocalStore and GDriveStore.
//
// The relPath returned by Save is an opaque token persisted verbatim in
// documents.stored_path and later handed back to Open/Delete. Its internal format
// is defined by the implementation and callers must not interpret it (LocalStore
// uses a filesystem-relative path; GDriveStore uses "gdrive:<fileID>").
type Store interface {
	Save(prID int64, originalName string, r io.Reader) (relPath string, size int64, err error)
	Open(relPath string) (io.ReadCloser, error)
	Delete(relPath string) error
}

var _ Store = (*LocalStore)(nil)
