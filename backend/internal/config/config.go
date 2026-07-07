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
