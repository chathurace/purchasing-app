-- Runtime-mutable file-storage settings, configured by an admin from
-- Settings → File storage (see docs/file-storage.md). A single row (id = 1)
-- holds the dynamic parts of the gdrive config: which folder and which Google
-- account (refresh token). The static GCP artifacts (OAuth client id/secret,
-- API key) stay in config.yaml. When this row is absent/empty, the server falls
-- back to the storage.* block in config.yaml — the CLI/bootstrap path.
--
-- refresh_token_enc is AES-256-GCM encrypted at rest (see internal/crypto).
CREATE TABLE storage_settings (
    id                   SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    backend              TEXT        NOT NULL DEFAULT 'gdrive',
    base_folder_id       TEXT        NOT NULL DEFAULT '',
    base_folder_name     TEXT        NOT NULL DEFAULT '',
    google_account_email TEXT        NOT NULL DEFAULT '',
    refresh_token_enc    TEXT        NOT NULL DEFAULT '',
    updated_by           BIGINT      REFERENCES users(id),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
