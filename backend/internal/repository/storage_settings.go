package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// StorageSettings is the single-row runtime file-storage configuration
// (storage_settings table). Exists is false when no row has been saved yet, in
// which case the server falls back to config.yaml.
type StorageSettings struct {
	Exists             bool
	Backend            string
	BaseFolderID       string
	BaseFolderName     string
	GoogleAccountEmail string
	RefreshTokenEnc    string
	UpdatedBy          *int64
	UpdatedAt          time.Time
}

// GetStorageSettings returns the saved settings, or Exists=false when none.
func (r *Repository) GetStorageSettings(ctx context.Context) (*StorageSettings, error) {
	s := &StorageSettings{}
	err := r.pool.QueryRow(ctx, `
		SELECT backend, base_folder_id, base_folder_name, google_account_email,
		       refresh_token_enc, updated_by, updated_at
		FROM storage_settings WHERE id = 1`).
		Scan(&s.Backend, &s.BaseFolderID, &s.BaseFolderName, &s.GoogleAccountEmail,
			&s.RefreshTokenEnc, &s.UpdatedBy, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return &StorageSettings{Exists: false}, nil
	}
	if err != nil {
		return nil, err
	}
	s.Exists = true
	return s, nil
}

// UpsertStorageSettings writes the single settings row.
func (r *Repository) UpsertStorageSettings(ctx context.Context, s StorageSettings, updatedBy int64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO storage_settings
			(id, backend, base_folder_id, base_folder_name, google_account_email, refresh_token_enc, updated_by, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (id) DO UPDATE SET
			backend = EXCLUDED.backend,
			base_folder_id = EXCLUDED.base_folder_id,
			base_folder_name = EXCLUDED.base_folder_name,
			google_account_email = EXCLUDED.google_account_email,
			refresh_token_enc = EXCLUDED.refresh_token_enc,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()`,
		s.Backend, s.BaseFolderID, s.BaseFolderName, s.GoogleAccountEmail, s.RefreshTokenEnc, updatedBy)
	return err
}
