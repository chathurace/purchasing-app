// Asgardeo's JWKS ships an x5c cert with a negative X.509 serial number, which
// Go 1.23+ rejects by default — go-jose fails to decode the key set and every
// OIDC token verification fails with "x509: negative serial number". The RSA
// key itself (n/e) is fine; only the attached cert is non-compliant. Restore
// the pre-1.23 behavior of tolerating it. See docs/oidc-asgardeo.md.
//
//go:debug x509negativeserial=1
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cs/purchasing-app/internal/config"
	"github.com/cs/purchasing-app/internal/crypto"
	"github.com/cs/purchasing-app/internal/email"
	"github.com/cs/purchasing-app/internal/handler"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/repository"
	"github.com/cs/purchasing-app/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

func main() {
	configFlag := flag.String("config", "", "path to config.yaml (default ./config.yaml or $PURCHASING_CONFIG)")
	flag.Parse()

	log := newLogger("server.log")

	cfg, err := config.Load(config.ConfigPath(*configFlag))
	if err != nil {
		log.Fatal().Err(err).Msg("load config")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		log.Fatal().Err(err).Msg("connect to postgres")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("ping postgres")
	}

	repo := repository.New(pool)

	// Secrets: encrypts the Drive refresh token stored via the in-app settings.
	// Optional — only needed once an admin connects Drive from the UI.
	var secrets *crypto.Secretbox
	if cfg.Security.SecretKey != "" {
		sb, err := crypto.New(cfg.Security.SecretKey)
		if err != nil {
			log.Fatal().Err(err).Msg("init security.secret_key")
		}
		secrets = sb
	}

	storageMgr := initStorage(ctx, cfg, repo, secrets, log)

	mailer := email.New(email.Config{
		Enabled:     cfg.Email.Enabled,
		SMTPHost:    cfg.Email.SMTPHost,
		SMTPPort:    cfg.Email.SMTPPort,
		Username:    cfg.Email.Username,
		Password:    cfg.Email.Password,
		FromAddress: cfg.Email.FromAddress,
	}, log)

	auth, err := middleware.NewAuth(ctx, middleware.AuthConfig{
		Issuer:              cfg.OIDC.Issuer,
		DiscoveryURL:        cfg.OIDC.DiscoveryURL,
		ClientID:            cfg.OIDC.ClientID,
		InsecureSkipVerify:  cfg.OIDC.InsecureSkipVerify,
		BootstrapAdminEmail: cfg.BootstrapAdmin.Email,
	}, repo, log)
	if err != nil {
		log.Fatal().Err(err).Msg("init OIDC")
	}

	router := handler.NewRouter(handler.Deps{
		Repo:           repo,
		Storage:        storageMgr,
		StorageManager: storageMgr,
		Secrets:        secrets,
		GDriveClientID: cfg.Storage.GDrive.OAuth.ClientID,
		GDriveAPIKey:   cfg.Storage.GDrive.OAuth.APIKey,
		GDriveAppID:    cfg.Storage.GDrive.OAuth.AppID,
		Auth:           auth,
		Mailer:         mailer,
		AppBaseURL:     cfg.Email.AppBaseURL,
		AllowedOrigins: cfg.CORS.AllowedOrigins,
		Log:            log,
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Int("port", cfg.Server.Port).Msg("server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down server")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

// initStorage builds the runtime-swappable StorageManager. For gdrive it prefers
// the saved in-app settings (storage_settings), then the config.yaml bootstrap,
// and otherwise boots unconfigured (an admin connects it from the UI). A storage
// misconfiguration never aborts startup — it surfaces as unhealthy status.
func initStorage(ctx context.Context, cfg *config.Config, repo *repository.Repository, secrets *crypto.Secretbox, log zerolog.Logger) *storage.StorageManager {
	gd := cfg.Storage.GDrive
	mgr := storage.NewManager(gd.OAuth.ClientID, gd.OAuth.ClientSecret)

	if cfg.Storage.Backend == "local" {
		ls, err := storage.NewLocalStore(cfg.Storage.FilesRoot)
		if err != nil {
			log.Fatal().Err(err).Msg("init local storage")
		}
		mgr.Set(ls, storage.Status{Backend: "local", BaseFolderName: ls.Root()})
		log.Info().Str("root", ls.Root()).Msg("local file storage initialized")
		return mgr
	}

	// gdrive: prefer saved settings from the DB.
	if settings, err := repo.GetStorageSettings(ctx); err != nil {
		log.Error().Err(err).Msg("read storage settings")
	} else if settings.Exists && settings.RefreshTokenEnc != "" && settings.BaseFolderID != "" {
		if secrets == nil {
			mgr.SetUnconfigured("gdrive", "saved credentials present but security.secret_key is not set in config.yaml")
			log.Warn().Msg("storage: security.secret_key missing; cannot use saved settings")
			return mgr
		}
		token, err := secrets.Decrypt(settings.RefreshTokenEnc)
		if err != nil {
			mgr.SetUnconfigured("gdrive", "saved credentials could not be decrypted (was security.secret_key changed?)")
			log.Error().Err(err).Msg("storage: decrypt saved refresh token")
			return mgr
		}
		if err := mgr.ReconfigureGDriveOAuth(ctx, token, settings.BaseFolderID, settings.GoogleAccountEmail); err != nil {
			log.Warn().Err(err).Msg("storage: configure from saved settings failed")
		} else {
			log.Info().Str("base_folder_id", settings.BaseFolderID).Str("account", settings.GoogleAccountEmail).Msg("gdrive storage configured from saved settings")
		}
		return mgr
	}

	// Fallback: config.yaml bootstrap (the CLI path).
	switch {
	case gd.Auth == "service_account" && gd.CredentialsFile != "" && gd.BaseFolderID != "":
		gs, err := storage.NewGDriveStore(ctx, gd.CredentialsFile, gd.BaseFolderID)
		if err != nil {
			mgr.SetUnconfigured("gdrive", err.Error())
			log.Warn().Err(err).Msg("storage: init service-account store")
			return mgr
		}
		mgr.Set(gs, storage.Status{Backend: "gdrive", BaseFolderID: gs.BaseFolderID(), BaseFolderName: gs.BaseFolderName()})
		log.Info().Str("base_folder_id", gs.BaseFolderID()).Str("auth", "service_account").Msg("google drive file storage initialized")
	case gd.Auth == "oauth" && gd.OAuth.RefreshToken != "" && gd.BaseFolderID != "":
		if err := mgr.ReconfigureGDriveOAuth(ctx, gd.OAuth.RefreshToken, gd.BaseFolderID, ""); err != nil {
			log.Warn().Err(err).Msg("storage: init oauth store from config.yaml")
		} else {
			log.Info().Str("base_folder_id", gd.BaseFolderID).Str("auth", "oauth").Msg("google drive file storage initialized")
		}
	default:
		mgr.SetUnconfigured("gdrive", "not configured — an admin can connect it in Settings → File storage")
		log.Warn().Msg("storage: gdrive not configured; awaiting admin setup")
	}
	return mgr
}

// newLogger writes to both stdout and ./logs/<name>.
func newLogger(name string) zerolog.Logger {
	logsDir := "logs"
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	f, err := os.OpenFile(filepath.Join(logsDir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return zerolog.New(os.Stdout).With().Timestamp().Logger()
	}
	w := io.MultiWriter(os.Stdout, f)
	return zerolog.New(w).With().Timestamp().Logger()
}
