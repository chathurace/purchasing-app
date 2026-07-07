package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// ErrNotConfigured is returned by the manager's Store methods when no backend has
// been configured yet (e.g. gdrive/oauth awaiting an admin to connect an account).
var ErrNotConfigured = errors.New("file storage is not configured — an admin must connect it in Settings → File storage")

// Status is a snapshot of the live storage configuration, for the settings UI.
type Status struct {
	Backend        string `json:"backend"`
	Configured     bool   `json:"configured"`
	Healthy        bool   `json:"healthy"`
	BaseFolderID   string `json:"base_folder_id,omitempty"`
	BaseFolderName string `json:"base_folder_name,omitempty"`
	AccountEmail   string `json:"account_email,omitempty"`
	LastError      string `json:"last_error,omitempty"`
}

// StorageManager implements Store by delegating to a current inner Store that can
// be hot-swapped at runtime. It also owns the static OAuth client credentials used
// to build gdrive/oauth stores and to exchange authorization codes. All handlers
// hold the manager (typed as Store), so a Reconfigure is transparent to them.
type StorageManager struct {
	clientID     string
	clientSecret string

	mu     sync.RWMutex
	cur    Store // nil when unconfigured
	status Status
}

var _ Store = (*StorageManager)(nil)

// NewManager returns an unconfigured manager holding the static OAuth client
// credentials (from config.yaml) used for the in-app connect + Picker flow.
func NewManager(clientID, clientSecret string) *StorageManager {
	return &StorageManager{clientID: clientID, clientSecret: clientSecret}
}

// Set installs a store built elsewhere (local, or the config.yaml bootstrap path)
// along with a status snapshot.
func (m *StorageManager) Set(s Store, status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cur = s
	status.Configured = true
	status.Healthy = true
	m.status = status
}

// SetUnconfigured records that storage could not be configured at startup, with a
// human-readable reason surfaced in the status UI.
func (m *StorageManager) SetUnconfigured(backend, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cur = nil
	m.status = Status{Backend: backend, Configured: false, Healthy: false, LastError: reason}
}

// ReconfigureGDriveOAuth builds a gdrive/oauth store from the given refresh token
// and base folder, validates it, and atomically swaps it in. On failure the
// previous store is kept and the error is returned (a bad config never takes
// storage down). accountEmail is used for status when the About lookup is empty.
func (m *StorageManager) ReconfigureGDriveOAuth(ctx context.Context, refreshToken, baseFolderID, accountEmail string) error {
	gs, err := NewGDriveStoreOAuth(ctx, m.clientID, m.clientSecret, refreshToken, baseFolderID)
	if err != nil {
		m.mu.Lock()
		m.status.LastError = err.Error()
		m.status.Healthy = m.cur != nil && m.status.Configured
		m.mu.Unlock()
		return err
	}
	email := gs.AccountEmail()
	if email == "" {
		email = accountEmail
	}
	m.Set(gs, Status{
		Backend:        "gdrive",
		BaseFolderID:   gs.BaseFolderID(),
		BaseFolderName: gs.BaseFolderName(),
		AccountEmail:   email,
	})
	return nil
}

// ExchangeCode exchanges a Google OAuth authorization code (from the browser code
// client) for a refresh token. redirectURL must match the one the code client
// used — "postmessage" for the GIS popup flow.
func (m *StorageManager) ExchangeCode(ctx context.Context, code, redirectURL string) (string, error) {
	if m.clientID == "" || m.clientSecret == "" {
		return "", errors.New("storage.gdrive.oauth client_id/client_secret are not configured in config.yaml")
	}
	conf := &oauth2.Config{
		ClientID:     m.clientID,
		ClientSecret: m.clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{drive.DriveFileScope},
	}
	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("exchange code: %w", err)
	}
	if tok.RefreshToken == "" {
		return "", errors.New("no refresh token returned — revoke prior access and retry with consent")
	}
	return tok.RefreshToken, nil
}

// ProbeAccount returns the email of the account a refresh token belongs to,
// without configuring storage — used right after connect, before a folder is
// picked, so the UI can show which account is connected.
func (m *StorageManager) ProbeAccount(ctx context.Context, refreshToken string) (string, error) {
	conf := &oauth2.Config{
		ClientID:     m.clientID,
		ClientSecret: m.clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveFileScope},
	}
	ts := conf.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	svc, err := drive.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", err
	}
	about, err := svc.About.Get().Fields("user(emailAddress)").Context(ctx).Do()
	if err != nil {
		return "", err
	}
	if about.User == nil {
		return "", nil
	}
	return about.User.EmailAddress, nil
}

// HasOAuthClient reports whether the static OAuth client credentials are present.
func (m *StorageManager) HasOAuthClient() bool {
	return m.clientID != "" && m.clientSecret != ""
}

// Status returns the current storage status snapshot.
func (m *StorageManager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *StorageManager) current() (Store, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cur == nil {
		return nil, ErrNotConfigured
	}
	return m.cur, nil
}

func (m *StorageManager) Save(prID int64, originalName string, r io.Reader) (string, int64, error) {
	s, err := m.current()
	if err != nil {
		return "", 0, err
	}
	return s.Save(prID, originalName, r)
}

func (m *StorageManager) Open(relPath string) (io.ReadCloser, error) {
	s, err := m.current()
	if err != nil {
		return nil, err
	}
	return s.Open(relPath)
}

func (m *StorageManager) Delete(relPath string) error {
	s, err := m.current()
	if err != nil {
		return err
	}
	return s.Delete(relPath)
}
