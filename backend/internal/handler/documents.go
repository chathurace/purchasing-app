package handler

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/rs/zerolog"
)

// pdfOnly restricts an upload to PDF (used for signed contract documents).
var pdfOnly = map[string]bool{".pdf": true}

// saveUploadedDoc parses a single multipart "file" field, validates its
// extension against allowed, stores it under the owning PR's directory, and
// records it as owned by (ownerType, ownerID). On any failure it writes the
// error response and returns ok=false. Mirrors PurchaseRequestsHandler.UploadDocument.
func saveUploadedDoc(
	w http.ResponseWriter, r *http.Request,
	repo *repository.Repository, store storage.Store, log zerolog.Logger,
	prID int64, ownerType string, ownerID int64, allowed map[string]bool,
) (*repository.Document, bool) {
	user := middleware.UserFromCtx(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1024)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid or too-large upload")
		return nil, false
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file field")
		return nil, false
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowed[ext] {
		writeError(w, http.StatusBadRequest, allowedMessage(allowed))
		return nil, false
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	storedPath, size, err := store.Save(prID, header.Filename, file)
	if err != nil {
		log.Error().Err(err).Msg("save document")
		writeError(w, http.StatusInternalServerError, "failed to store file")
		return nil, false
	}
	doc, err := repo.AddOwnedDocument(r.Context(), prID, ownerType, ownerID, header.Filename, storedPath, contentType, size, user.ID, strings.TrimSpace(r.FormValue("notes")))
	if err != nil {
		_ = store.Delete(storedPath)
		log.Error().Err(err).Msg("record document")
		writeError(w, http.StatusInternalServerError, "failed to record file")
		return nil, false
	}
	return doc, true
}

// downloadOwnedDoc streams an owned document with its original filename.
func downloadOwnedDoc(
	w http.ResponseWriter, r *http.Request,
	repo *repository.Repository, store storage.Store, log zerolog.Logger,
	ownerType string, ownerID, docID int64,
) {
	doc, err := repo.GetOwnedDocument(r.Context(), ownerType, ownerID, docID)
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	f, err := store.Open(doc.StoredPath)
	if err != nil {
		log.Error().Err(err).Str("path", doc.StoredPath).Msg("open document")
		writeError(w, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", doc.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeHeaderFilename(doc.Filename)+"\"")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, f); err != nil {
		log.Warn().Err(err).Msg("stream document")
	}
}

// deleteOwnedDoc removes an owned document row and its stored file.
func deleteOwnedDoc(
	w http.ResponseWriter, r *http.Request,
	repo *repository.Repository, store storage.Store,
	ownerType string, ownerID, docID int64,
) {
	doc, err := repo.GetOwnedDocument(r.Context(), ownerType, ownerID, docID)
	if err != nil {
		writeError(w, http.StatusNotFound, "document not found")
		return
	}
	if err := repo.DeleteOwnedDocument(r.Context(), ownerType, ownerID, docID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete document")
		return
	}
	_ = store.Delete(doc.StoredPath)
	w.WriteHeader(http.StatusNoContent)
}

func allowedMessage(allowed map[string]bool) string {
	if len(allowed) == 1 && allowed[".pdf"] {
		return "only .pdf files are allowed"
	}
	return "only .pdf and .docx files are allowed"
}
