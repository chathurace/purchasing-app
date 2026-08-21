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
	"github.com/cs/purchasing-app/internal/directory"
	"github.com/cs/purchasing-app/internal/email"
	"github.com/cs/purchasing-app/internal/extraction"
	"github.com/cs/purchasing-app/internal/handler"
	"github.com/cs/purchasing-app/internal/middleware"
	"github.com/cs/purchasing-app/internal/offboard"
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

	// Backend-issued browser sessions (migration 052, docs/sessions.md). The SPA
	// trades its IdP token for an HttpOnly cookie once, then authenticates with
	// that cookie for session.ttl_days of idle time — so how long a user stays
	// signed in is this app's decision, not Asgardeo's.
	sessionCfg := middleware.SessionConfig{
		Enabled:        *cfg.Session.Enabled,
		TTL:            time.Duration(cfg.Session.TTLDays) * 24 * time.Hour,
		MaxLifetime:    time.Duration(*cfg.Session.MaxDays) * 24 * time.Hour,
		Secure:         *cfg.Session.CookieSecure,
		SameSite:       sameSiteFromConfig(cfg.Session.CookieSameSite),
		AllowedOrigins: cfg.CORS.AllowedOrigins,
	}
	if sessionCfg.Enabled {
		log.Info().
			Dur("ttl", sessionCfg.TTL).
			Dur("max_lifetime", sessionCfg.MaxLifetime).
			Str("samesite", cfg.Session.CookieSameSite).
			Bool("secure", sessionCfg.Secure).
			Str("cookie", sessionCfg.CookieName()).
			Msg("cookie sessions enabled")
		if sessionCfg.MaxLifetime <= 0 {
			// Deliberate but worth shouting about: with no ceiling, a session
			// used at least once per idle window never expires on its own, so an
			// IdP-side disable is only honoured once someone revokes by hand.
			log.Warn().Msg("session.max_days is 0 — sessions have NO absolute lifetime cap")
		}
	} else {
		log.Warn().Msg("cookie sessions disabled — clients must present a Bearer token on every request")
	}

	auth, err := middleware.NewAuth(ctx, middleware.AuthConfig{
		Issuer:              cfg.OIDC.Issuer,
		DiscoveryURL:        cfg.OIDC.DiscoveryURL,
		ClientID:            cfg.OIDC.ClientID,
		InsecureSkipVerify:  cfg.OIDC.InsecureSkipVerify,
		BootstrapAdminEmail: cfg.BootstrapAdmin.Email,
		Session:             sessionCfg,
	}, repo, log)
	if err != nil {
		log.Fatal().Err(err).Msg("init OIDC")
	}
	if sessionCfg.Enabled {
		go pruneSessions(ctx, repo, sessionCfg.MaxLifetime, log)
	}

	// User directory for name/email autocomplete. When SCIM is disabled, it falls
	// back to the app's own active DB users so dev needs no M2M app.
	dirSvc := directory.New(
		cfg.SCIM.Enabled,
		directory.SCIMConfig{
			BaseURL:            cfg.SCIM.BaseURL,
			TokenURL:           cfg.SCIM.TokenURL,
			ClientID:           cfg.SCIM.ClientID,
			ClientSecret:       cfg.SCIM.ClientSecret,
			Scopes:             cfg.SCIM.Scopes,
			InsecureSkipVerify: cfg.SCIM.InsecureSkipVerify,
			PageSize:           cfg.SCIM.PageSize,
		},
		time.Duration(cfg.SCIM.CacheTTLSeconds)*time.Second,
		func(ctx context.Context) ([]directory.User, error) {
			summaries, err := repo.ListActiveUsers(ctx)
			if err != nil {
				return nil, err
			}
			users := make([]directory.User, 0, len(summaries))
			for _, s := range summaries {
				if s.Email == "" {
					continue
				}
				// Active: these are already the app's *active* users. It also
				// keeps the fallback from ever looking like a directory full of
				// disabled accounts, though the offboarding sweep refuses to run
				// on fallback data anyway (it requires SCIM).
				users = append(users, directory.User{Name: s.Name, Email: s.Email, Active: true})
			}
			return users, nil
		},
		log,
	)
	log.Info().Bool("scim_enabled", dirSvc.Enabled()).Msg("user directory initialized")

	// Sign out users the identity server no longer vouches for (deleted or
	// disabled accounts). Pull-based, riding the directory cache above — see
	// internal/offboard for why this exists rather than back-channel logout.
	if sessionCfg.Enabled && *cfg.Session.IdPOffboarding.Enabled {
		sweeper := offboard.New(repo, dirSvc, offboard.Config{
			Interval: time.Duration(cfg.Session.IdPOffboarding.IntervalMinutes) * time.Minute,
			DryRun:   cfg.Session.IdPOffboarding.DryRun,
		}, log)
		go sweeper.Run(ctx)
	} else if sessionCfg.Enabled {
		log.Warn().Msg("idp offboarding sweep is disabled in config — a disabled IdP account keeps its app session until an admin ends it or session.max_days lapses")
	}

	// Quotation PDF extraction (Claude). Disabled unless anthropic.enabled is set,
	// in which case the endpoints 503 and the UI hides the feature.
	extractSvc := extraction.New(extraction.Config{
		Enabled:     cfg.Anthropic.Enabled,
		APIKey:      cfg.Anthropic.APIKey,
		Model:       cfg.Anthropic.Model,
		MaxPDFBytes: cfg.Anthropic.MaxPDFBytes,
		Timeout:     time.Duration(cfg.Anthropic.TimeoutSeconds) * time.Second,
	}, log)
	log.Info().
		Bool("extraction_enabled", extractSvc.Enabled()).
		Str("model", extractSvc.Model()).
		Msg("quotation extraction initialized")

	router := handler.NewRouter(handler.Deps{
		Repo:           repo,
		Storage:        storageMgr,
		StorageManager: storageMgr,
		Secrets:        secrets,
		GDriveClientID: cfg.Storage.GDrive.OAuth.ClientID,
		GDriveAPIKey:   cfg.Storage.GDrive.OAuth.APIKey,
		GDriveAppID:    cfg.Storage.GDrive.OAuth.AppID,
		Auth:           auth,
		Directory:      dirSvc,
		Extraction:     extractSvc,
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

// sameSiteFromConfig maps the config.yaml session.cookie_samesite value onto
// Go's enum. config.Load has already rejected anything else.
func sameSiteFromConfig(v string) http.SameSite {
	switch v {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}

// pruneSessions deletes expired, long-revoked and over-cap session rows — once
// at boot, then daily. Nothing depends on this for correctness (TouchUserSession
// enforces both deadlines itself); it only keeps the table from growing without
// bound.
func pruneSessions(ctx context.Context, repo *repository.Repository, maxLifetime time.Duration, log zerolog.Logger) {
	prune := func() {
		pruneCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		n, err := repo.DeleteExpiredUserSessions(pruneCtx, maxLifetime)
		if err != nil {
			log.Error().Err(err).Msg("session cleanup")
			return
		}
		if n > 0 {
			log.Info().Int64("deleted", n).Msg("pruned expired/revoked sessions")
		}
	}
	prune()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
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
