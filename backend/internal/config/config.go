package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the full application configuration, loaded from a YAML file.
type Config struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`

	Database struct {
		URL string `yaml:"url"`
	} `yaml:"database"`

	OIDC struct {
		Issuer             string `yaml:"issuer"`
		DiscoveryURL       string `yaml:"discovery_url"`
		ClientID           string `yaml:"client_id"`
		InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
	} `yaml:"oidc"`

	// Session configures the backend-issued browser session cookie — the app's
	// own session, independent of the IdP's token lifetimes (docs/sessions.md).
	// The SPA logs in through the IdP once and exchanges that token for an
	// HttpOnly cookie good for TTLDays of *idle* time, which is what stops the
	// roughly-daily sign-outs. Enabled defaults to true; set `enabled: false` to
	// fall back to Bearer-token-only auth (the pre-session behaviour).
	Session struct {
		Enabled *bool `yaml:"enabled"`  // default true
		TTLDays int   `yaml:"ttl_days"` // idle lifetime; default 60
		// MaxDays is the absolute ceiling measured from when the session was
		// created, regardless of activity. Default 90; an explicit 0 disables it
		// (logged as a warning at startup), hence the pointer — nil means "not
		// set in the YAML" and must be distinguishable from a deliberate 0.
		// Without a cap the sliding idle window lets a session that is used often
		// enough live forever — and because the IdP is never consulted after the
		// cookie is minted, an account disabled at Asgardeo would keep working
		// for exactly that long. See docs/sessions.md.
		MaxDays *int `yaml:"max_days"`
		// CookieSecure defaults to true. Set false ONLY for local dev over
		// http://localhost — a Secure cookie is not stored on a plain-HTTP page
		// by every browser, and the __Host- name prefix requires Secure.
		CookieSecure *bool `yaml:"cookie_secure"`
		// CookieSameSite is "lax" (default) when the API is same-site with the
		// SPA — localhost:5173 → localhost:8081 in dev, or a proxied /api in
		// production — and "none" when the browser talks to the backend
		// cross-site (e.g. a Choreo web app calling a Choreo API on another
		// domain). "none" implies Secure and a Partitioned cookie, and is
		// dropped outright by browsers that block third-party cookies.
		CookieSameSite string `yaml:"cookie_samesite"`
	} `yaml:"session"`

	// SCIM configures the connection to the identity server's SCIM2 API, used to
	// populate user-autocomplete suggestions from the org directory. When
	// disabled (the default), directory lookups fall back to the local DB users,
	// so dev runs without an M2M app. The client authenticates with the OAuth2
	// client-credentials grant (a machine-to-machine app in WSO2 IS / Asgardeo
	// holding the `internal_user_mgt_list` scope).
	SCIM struct {
		Enabled            bool     `yaml:"enabled"`
		BaseURL            string   `yaml:"base_url"`  // SCIM2 base, e.g. https://localhost:9443/scim2
		TokenURL           string   `yaml:"token_url"` // token endpoint, e.g. https://localhost:9443/oauth2/token
		ClientID           string   `yaml:"client_id"`
		ClientSecret       string   `yaml:"client_secret"`
		Scopes             []string `yaml:"scopes"`
		InsecureSkipVerify bool     `yaml:"insecure_skip_verify"` // dev only — trusts self-signed cert
		CacheTTLSeconds    int      `yaml:"cache_ttl_seconds"`    // directory cache lifetime; default 600
		PageSize           int      `yaml:"page_size"`            // SCIM page size; default 100
	} `yaml:"scim"`

	// Anthropic configures Claude-backed quotation PDF extraction (reading vendor
	// / currency / total / line items out of an uploaded quotation). When disabled
	// (the default) the extraction endpoints report unavailable and the UI hides
	// the feature, so dev and CI run without an API key. The key is server-side
	// only — it is never sent to the browser.
	Anthropic struct {
		Enabled        bool   `yaml:"enabled"`
		APIKey         string `yaml:"api_key"`
		Model          string `yaml:"model"`           // default claude-opus-5
		MaxPDFBytes    int64  `yaml:"max_pdf_bytes"`   // default 20MB
		TimeoutSeconds int    `yaml:"timeout_seconds"` // per-extraction cap; default 180
	} `yaml:"anthropic"`

	Storage struct {
		Backend   string `yaml:"backend"` // "local" (default) or "gdrive"
		FilesRoot string `yaml:"files_root"`
		GDrive    struct {
			// Auth selects how the app authenticates to Drive:
			//   "service_account" (default) — CredentialsFile, app acts as itself
			//   "oauth"                      — OAuth as a real user (OAuth block)
			Auth string `yaml:"auth"`
			// service_account
			CredentialsFile string `yaml:"credentials_file"`
			// oauth — the app acts as the user who granted this refresh token.
			// Mint the refresh token once with `go run ./cmd/gdrive-auth`, or let
			// an admin connect from Settings → File storage (persisted in the
			// storage_settings table). client_id/client_secret/api_key are the
			// static GCP artifacts that enable the in-app connect + Picker flow;
			// refresh_token here is the bootstrap/fallback used when no DB row
			// exists. app_id defaults to the client_id prefix (GCP project number).
			OAuth struct {
				ClientID     string `yaml:"client_id"`
				ClientSecret string `yaml:"client_secret"`
				APIKey       string `yaml:"api_key"`
				AppID        string `yaml:"app_id"`
				RefreshToken string `yaml:"refresh_token"`
			} `yaml:"oauth"`
			BaseFolderID string `yaml:"base_folder_id"`
		} `yaml:"gdrive"`
	} `yaml:"storage"`

	// Security holds app-level secrets. SecretKey (32-byte base64) encrypts
	// sensitive values at rest (currently the Drive refresh token stored via the
	// in-app storage settings). Optional: only required once an admin connects
	// Drive from the UI (or when a storage_settings row already holds a token).
	Security struct {
		SecretKey string `yaml:"secret_key"`
	} `yaml:"security"`

	BootstrapAdmin struct {
		Email string `yaml:"email"`
	} `yaml:"bootstrap_admin"`

	CORS struct {
		AllowedOrigins []string `yaml:"allowed_origins"`
	} `yaml:"cors"`

	// Email is optional. When disabled (the default), approval notifications are
	// logged instead of sent, so the app runs without an SMTP server in dev.
	Email struct {
		Enabled     bool   `yaml:"enabled"`
		SMTPHost    string `yaml:"smtp_host"`
		SMTPPort    int    `yaml:"smtp_port"`
		Username    string `yaml:"username"`
		Password    string `yaml:"password"`
		FromAddress string `yaml:"from_address"`
		// AppBaseURL is the frontend origin used to build links in emails
		// (e.g. https://purchasing.example.com). Falls back to the first CORS
		// origin when empty.
		AppBaseURL string `yaml:"app_base_url"`
	} `yaml:"email"`
}

// Load reads and parses the YAML config at path, applies defaults, and validates
// required fields.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8081
	}
	if cfg.Storage.FilesRoot == "" {
		cfg.Storage.FilesRoot = "../files"
	}
	if cfg.Storage.Backend == "" {
		cfg.Storage.Backend = "local"
	}
	if cfg.Email.SMTPPort == 0 {
		cfg.Email.SMTPPort = 587
	}
	// Sessions are on by default (an absent `session:` block means "60-day
	// cookie sessions, Secure, Lax"), so an existing deployment gets the fix
	// without a config edit.
	if cfg.Session.Enabled == nil {
		cfg.Session.Enabled = boolPtr(true)
	}
	if cfg.Session.TTLDays <= 0 {
		cfg.Session.TTLDays = 60
	}
	// Absent (nil) means "use the default"; an explicit 0 disables the cap. A
	// negative value is meaningless, so it falls back to the default too.
	if cfg.Session.MaxDays == nil || *cfg.Session.MaxDays < 0 {
		cfg.Session.MaxDays = intPtr(90)
	}
	if cfg.Session.CookieSecure == nil {
		cfg.Session.CookieSecure = boolPtr(true)
	}
	if cfg.Session.CookieSameSite == "" {
		cfg.Session.CookieSameSite = "lax"
	}
	if cfg.SCIM.CacheTTLSeconds == 0 {
		cfg.SCIM.CacheTTLSeconds = 600
	}
	if cfg.SCIM.PageSize == 0 {
		cfg.SCIM.PageSize = 100
	}
	if cfg.Anthropic.Model == "" {
		cfg.Anthropic.Model = "claude-opus-5"
	}
	if cfg.Anthropic.MaxPDFBytes == 0 {
		cfg.Anthropic.MaxPDFBytes = 20 << 20
	}
	if cfg.Anthropic.TimeoutSeconds == 0 {
		cfg.Anthropic.TimeoutSeconds = 180
	}
	if cfg.Email.AppBaseURL == "" && len(cfg.CORS.AllowedOrigins) > 0 {
		cfg.Email.AppBaseURL = cfg.CORS.AllowedOrigins[0]
	}

	// Validation
	if cfg.Database.URL == "" {
		return nil, fmt.Errorf("database.url is required")
	}
	if cfg.OIDC.Issuer == "" {
		return nil, fmt.Errorf("oidc.issuer is required")
	}
	if cfg.OIDC.ClientID == "" {
		return nil, fmt.Errorf("oidc.client_id is required")
	}
	switch cfg.Session.CookieSameSite {
	case "lax", "strict":
	case "none":
		// SameSite=None is only honoured on a Secure cookie; without this the
		// browser silently drops it and the session looks broken.
		if !*cfg.Session.CookieSecure {
			return nil, fmt.Errorf("session.cookie_secure must be true when session.cookie_samesite is \"none\"")
		}
	default:
		return nil, fmt.Errorf("session.cookie_samesite %q invalid (want \"lax\", \"none\" or \"strict\")", cfg.Session.CookieSameSite)
	}
	if cfg.SCIM.Enabled {
		if cfg.SCIM.BaseURL == "" {
			return nil, fmt.Errorf("scim.base_url is required when scim.enabled is true")
		}
		if cfg.SCIM.TokenURL == "" {
			return nil, fmt.Errorf("scim.token_url is required when scim.enabled is true")
		}
		if cfg.SCIM.ClientID == "" || cfg.SCIM.ClientSecret == "" {
			return nil, fmt.Errorf("scim.client_id and scim.client_secret are required when scim.enabled is true")
		}
	}
	if cfg.Anthropic.Enabled && cfg.Anthropic.APIKey == "" {
		return nil, fmt.Errorf("anthropic.api_key is required when anthropic.enabled is true")
	}
	switch cfg.Storage.Backend {
	case "local":
		// files_root already defaulted above
	case "gdrive":
		if cfg.Storage.GDrive.Auth == "" {
			cfg.Storage.GDrive.Auth = "service_account"
		}
		switch cfg.Storage.GDrive.Auth {
		case "service_account":
			if cfg.Storage.GDrive.CredentialsFile == "" {
				return nil, fmt.Errorf("storage.gdrive.credentials_file is required when storage.gdrive.auth=service_account")
			}
			if cfg.Storage.GDrive.BaseFolderID == "" {
				return nil, fmt.Errorf("storage.gdrive.base_folder_id is required when storage.gdrive.auth=service_account")
			}
			if _, err := os.Stat(cfg.Storage.GDrive.CredentialsFile); err != nil {
				return nil, fmt.Errorf("storage.gdrive.credentials_file %q not readable: %w", cfg.Storage.GDrive.CredentialsFile, err)
			}
		case "oauth":
			// All oauth fields are optional at load time. The folder + refresh
			// token may be configured at runtime by an admin (Settings → File
			// storage, persisted in storage_settings); config.yaml values, when
			// present, act as the bootstrap/fallback. The StorageManager decides
			// whether storage is actually configured and surfaces that as status.
		default:
			return nil, fmt.Errorf("storage.gdrive.auth %q invalid (want \"service_account\" or \"oauth\")", cfg.Storage.GDrive.Auth)
		}
	default:
		return nil, fmt.Errorf("storage.backend %q invalid (want \"local\" or \"gdrive\")", cfg.Storage.Backend)
	}

	return &cfg, nil
}

// boolPtr / intPtr are the defaulting helpers for the pointer config fields,
// where nil means "not set in the YAML" and so must be distinguishable from an
// explicit false / 0.
func boolPtr(v bool) *bool { return &v }
func intPtr(v int) *int    { return &v }

// ConfigPath resolves the config file path: the -config flag value if given,
// else the PURCHASING_CONFIG env var, else ./config.yaml.
func ConfigPath(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if v := os.Getenv("PURCHASING_CONFIG"); v != "" {
		return v
	}
	return "config.yaml"
}
