package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	gdriveFolderMime = "application/vnd.google-apps.folder"
	gdrivePrefix     = "gdrive:"

	// Operation timeouts. Folder lookups/deletes are quick; uploads and downloads
	// may involve large files over slow links.
	gdriveMetaTimeout = 60 * time.Second
	gdriveBlobTimeout = 10 * time.Minute
)

// GDriveStore stores files in a Google Shared Drive, one subfolder per purchase
// request (named by the PR id) under a configured base folder:
//
//	<baseFolderID>/<prID>/<originalFilename>
//
// Files are addressed by their Drive file ID; Save returns "gdrive:<fileID>",
// persisted verbatim in documents.stored_path and handed back to Open/Delete.
type GDriveStore struct {
	svc            *drive.Service
	baseFolderID   string
	baseFolderName string // resolved display name of the base folder, for status
	accountEmail   string // authenticated user's email (oauth mode), for status
	driveID        string // enclosing Shared Drive ID (empty for My Drive), for scoping List queries

	mu    sync.Mutex       // serializes find-or-create of per-PR folders
	cache map[int64]string // prID -> folder file ID
}

// NewGDriveStore builds a Drive client from a service-account key file and
// validates that the base folder is visible to the service account.
func NewGDriveStore(ctx context.Context, credentialsFile, baseFolderID string) (*GDriveStore, error) {
	// Full drive scope (not drive.file): the base folder is created by a human in
	// the Drive UI, so drive.file — which only sees app-created files — could not
	// read it.
	svc, err := drive.NewService(ctx,
		option.WithCredentialsFile(credentialsFile),
		option.WithScopes(drive.DriveScope),
	)
	if err != nil {
		return nil, fmt.Errorf("init drive service: %w", err)
	}
	return newGDriveStore(ctx, svc, baseFolderID)
}

// NewGDriveStoreOAuth builds a Drive client that acts as a real user, via an
// OAuth refresh token minted once with `cmd/gdrive-auth`. Unlike a service
// account, the user has personal Drive quota and inherits their own access to
// corporate Shared Drives — so this works where org policy blocks adding a
// service account as a Shared Drive member. The oauth2 token source refreshes
// access tokens automatically from the refresh token.
//
// Scope is the fine-grained drive.file (NOT full drive): the app can only touch
// files it created plus the base folder the user granted once via the Google
// Picker (`cmd/gdrive-grant`). Nothing else in the user's Drive is reachable.
// This store only ever creates its own PR folders/files under the base folder —
// which drive.file always permits — so it never needs broader access. The base
// folder resolves at startup only because the Picker grant is already in place;
// a 404 here means the grant is missing or was done as a different user/client.
func NewGDriveStoreOAuth(ctx context.Context, clientID, clientSecret, refreshToken, baseFolderID string) (*GDriveStore, error) {
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveFileScope},
	}
	ts := conf.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	svc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("init drive service: %w", err)
	}
	s, err := newGDriveStore(ctx, svc, baseFolderID)
	if err != nil {
		return nil, err
	}
	// Best-effort: record which account this token belongs to, for the status UI.
	if about, aerr := svc.About.Get().Fields("user(emailAddress)").Context(ctx).Do(); aerr == nil && about.User != nil {
		s.accountEmail = about.User.EmailAddress
	}
	return s, nil
}

// newGDriveStore resolves the base folder once (shared by both auth modes). This
// fails fast on the #1 setup mistake — the authenticated identity not being able
// to see the base folder — and captures the enclosing Shared Drive ID for List
// scoping.
func newGDriveStore(ctx context.Context, svc *drive.Service, baseFolderID string) (*GDriveStore, error) {
	base, err := svc.Files.Get(baseFolderID).
		SupportsAllDrives(true).
		Fields("id", "name", "driveId", "mimeType", "capabilities(canAddChildren)").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("resolve base folder %q (check the folder is visible to the authenticated identity — the service account or OAuth user): %w", baseFolderID, err)
	}
	if base.MimeType != gdriveFolderMime {
		return nil, fmt.Errorf("storage.gdrive.base_folder_id %q is not a folder", baseFolderID)
	}
	// Fail fast if the account can see the folder but only with view access —
	// uploads would otherwise fail later with a confusing 403.
	if base.Capabilities != nil && !base.Capabilities.CanAddChildren {
		return nil, fmt.Errorf("base folder %q is not writable by the authenticated account (no create access)", baseFolderID)
	}

	return &GDriveStore{
		svc:            svc,
		baseFolderID:   baseFolderID,
		baseFolderName: base.Name,
		driveID:        base.DriveId,
		cache:          make(map[int64]string),
	}, nil
}

// BaseFolderID returns the configured base folder ID (used only for the startup log).
func (s *GDriveStore) BaseFolderID() string { return s.baseFolderID }

// BaseFolderName returns the resolved display name of the base folder.
func (s *GDriveStore) BaseFolderName() string { return s.baseFolderName }

// AccountEmail returns the authenticated user's email (oauth mode; empty otherwise).
func (s *GDriveStore) AccountEmail() string { return s.accountEmail }

// folderID returns the file ID of the per-PR folder, creating it if absent. The
// whole find-or-create runs under a mutex so concurrent uploads to a new PR do
// not create duplicate folders; results are cached to skip the List round-trip
// on repeat uploads.
func (s *GDriveStore) folderID(prID int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.cache[prID]; ok {
		return id, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), gdriveMetaTimeout)
	defer cancel()

	name := fmt.Sprintf("%d", prID)
	q := fmt.Sprintf("name = '%d' and mimeType = '%s' and '%s' in parents and trashed = false",
		prID, gdriveFolderMime, s.baseFolderID)

	list := s.svc.Files.List().
		Q(q).
		Spaces("drive").
		SupportsAllDrives(true).
		IncludeItemsFromAllDrives(true).
		Fields("files(id,name)").
		Context(ctx)
	if s.driveID != "" {
		list = list.Corpora("drive").DriveId(s.driveID)
	}

	res, err := list.Do()
	if err != nil {
		return "", fmt.Errorf("list pr folder: %w", err)
	}
	if len(res.Files) > 0 {
		id := res.Files[0].Id // graceful degradation if duplicates ever exist
		s.cache[prID] = id
		return id, nil
	}

	created, err := s.svc.Files.Create(&drive.File{
		Name:     name,
		MimeType: gdriveFolderMime,
		Parents:  []string{s.baseFolderID},
	}).
		SupportsAllDrives(true).
		Fields("id").
		Context(ctx).
		Do()
	if err != nil {
		return "", fmt.Errorf("create pr folder: %w", err)
	}
	s.cache[prID] = created.Id
	return created.Id, nil
}

// Save streams r into a new Drive file in the PR's folder, preserving the
// original filename. Duplicate display names are fine — files are addressed by
// ID. Returns "gdrive:<fileID>" and the stored byte size.
func (s *GDriveStore) Save(prID int64, originalName string, r io.Reader) (string, int64, error) {
	parent, err := s.folderID(prID)
	if err != nil {
		return "", 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), gdriveBlobTimeout)
	defer cancel()

	cr := &countingReader{r: r}
	f, err := s.svc.Files.Create(&drive.File{
		Name:    sanitizeName(originalName),
		Parents: []string{parent},
	}).
		Media(cr, googleapi.ContentType("application/octet-stream")).
		SupportsAllDrives(true).
		Fields("id", "size").
		Context(ctx).
		Do()
	if err != nil {
		return "", 0, fmt.Errorf("upload file: %w", err)
	}

	size := f.Size
	if size == 0 {
		size = cr.n
	}
	return gdrivePrefix + f.Id, size, nil
}

// Open opens a stored file for reading. The returned ReadCloser owns a context
// that is cancelled when it is closed.
func (s *GDriveStore) Open(relPath string) (io.ReadCloser, error) {
	id := fileID(relPath)

	ctx, cancel := context.WithTimeout(context.Background(), gdriveBlobTimeout)
	resp, err := s.svc.Files.Get(id).
		SupportsAllDrives(true).
		Context(ctx).
		Download()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("download file: %w", err)
	}
	return &cancelReadCloser{ReadCloser: resp.Body, cancel: cancel}, nil
}

// Delete removes a stored file. A missing file is not an error, matching
// LocalStore, so save-rollback and delete paths are idempotent.
func (s *GDriveStore) Delete(relPath string) error {
	id := fileID(relPath)

	ctx, cancel := context.WithTimeout(context.Background(), gdriveMetaTimeout)
	defer cancel()

	err := s.svc.Files.Delete(id).
		SupportsAllDrives(true).
		Context(ctx).
		Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

// fileID strips the "gdrive:" prefix from a stored path to recover the Drive ID.
func fileID(relPath string) string {
	if len(relPath) >= len(gdrivePrefix) && relPath[:len(gdrivePrefix)] == gdrivePrefix {
		return relPath[len(gdrivePrefix):]
	}
	return relPath
}

// countingReader counts bytes read through it, as a fallback for the stored size.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// cancelReadCloser cancels its context when closed, tying a download's lifetime
// to the caller's use of the body.
type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

var _ Store = (*GDriveStore)(nil)
