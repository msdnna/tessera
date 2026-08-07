// Package config loads and validates runtime configuration from the environment.
package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config holds the runtime configuration loaded from the environment.
type Config struct {
	DatabaseURL   string
	Port          string
	JWTSecret     string
	AppEnv        string
	UploadDir     string
	EncryptionKey string
	// SMTP / email (scaffolded in U1; flows land in U2). All optional — when
	// unset, mail.New returns a no-op mailer. PublicURL is the externally
	// reachable base URL used to build links (verification / invite / reset).
	SMTPHost  string
	SMTPPort  string
	SMTPUser  string
	SMTPPass  string
	SMTPFrom  string
	PublicURL string
	// Origin allowed by the CORS middleware. In production defaults to
	// PublicURL (lock the API to the web app's origin); in dev defaults to "*".
	// Override explicitly with CORS_ORIGIN.
	CORSOrigin string
	// Additional allowed origins for the desktop (Tauri) app, reflected by the
	// CORS middleware alongside CORSOrigin. The Tauri webview origin differs per
	// OS (Windows WebView2 ≈ http://tauri.localhost; Linux WebKitGTK ≈
	// tauri://localhost), so all known forms are allowed by default. Override
	// with DESKTOP_CORS_ORIGINS (comma-separated).
	DesktopOrigins []string
	// Budget for draining in-flight requests and background workers after
	// SIGTERM/SIGINT before the process exits anyway (GRACEFUL_TIMEOUT).
	GracefulTimeout time.Duration
	// How often a personal access token's last_used_at is actually written.
	// Touching on every request costs a pool connection per call for a value
	// nobody reads at second precision (PAT_TOUCH_INTERVAL; 0 disables the
	// throttle and writes on every request).
	PATTouchInterval time.Duration
}

// New reads configuration from the environment. In production
// (APP_ENV=production) JWT_SECRET and DATABASE_URL must be explicitly set —
// fail-closed prevents the binary from booting with dev defaults.
func New() *Config {
	prod := strings.EqualFold(os.Getenv("APP_ENV"), "production")

	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		if prod {
			log.Fatal("JWT_SECRET is required in production (APP_ENV=production)")
		}
		jwt = "dev-secret-change-in-production-min32chars!"
		log.Println("WARNING: JWT_SECRET not set — using dev default (do NOT use in production)")
	} else if len(jwt) < 32 {
		log.Printf("WARNING: JWT_SECRET is shorter than 32 chars (%d) — generate a stronger one", len(jwt))
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// Compose sets DATABASE_URL for the backend container, but host-run
		// tooling (cmd/migrate) usually has only POSTGRES_* in .env. Stitch a
		// URL from those, defaulting to localhost:5432 (the exposed dev port).
		if user := os.Getenv("POSTGRES_USER"); user != "" {
			pass := os.Getenv("POSTGRES_PASSWORD")
			host := getEnv("POSTGRES_HOST", "localhost")
			port := getEnv("POSTGRES_PORT", "5432")
			name := getEnv("POSTGRES_DB", "tessera")
			dbURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
				url.QueryEscape(user), url.QueryEscape(pass), host, port, name)
		}
	}
	if dbURL == "" {
		if prod {
			log.Fatal("DATABASE_URL is required in production (APP_ENV=production)")
		}
		dbURL = "postgres://tessera:tessera@localhost:5432/tessera?sslmode=disable"
		log.Println("WARNING: DATABASE_URL not set — using local dev default")
	}

	// Key for encrypting secrets at rest (GitLab PATs). Independent from
	// JWT_SECRET so rotating one doesn't invalidate the other.
	encKey := os.Getenv("ENCRYPTION_KEY")
	if encKey == "" {
		if prod {
			log.Fatal("ENCRYPTION_KEY is required in production (APP_ENV=production)")
		}
		encKey = "dev-encryption-key-change-in-production"
		log.Println("WARNING: ENCRYPTION_KEY not set — using dev default (stored secrets won't decrypt in prod)")
	}

	// CORS origin: explicit CORS_ORIGIN wins; else lock to PublicURL in prod,
	// or fall back to "*" for dev convenience.
	publicURL := os.Getenv("PUBLIC_URL")
	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		if prod && publicURL != "" {
			corsOrigin = publicURL
		} else {
			corsOrigin = "*"
		}
	}

	// Desktop (Tauri) origins reflected in addition to CORSOrigin. Ignored when
	// CORSOrigin is a wildcard (dev), where every origin is already allowed.
	desktopOrigins := splitCSV(getEnv("DESKTOP_CORS_ORIGINS",
		"tauri://localhost,http://tauri.localhost,https://tauri.localhost"))

	return &Config{
		DatabaseURL:    dbURL,
		Port:           getEnv("PORT", "8080"),
		JWTSecret:      jwt,
		AppEnv:         getEnv("APP_ENV", "development"),
		UploadDir:      getEnv("UPLOAD_DIR", "./uploads"),
		EncryptionKey:  encKey,
		SMTPHost:       os.Getenv("SMTP_HOST"),
		SMTPPort:       getEnv("SMTP_PORT", "587"),
		SMTPUser:       os.Getenv("SMTP_USER"),
		SMTPPass:       os.Getenv("SMTP_PASS"),
		SMTPFrom:       os.Getenv("SMTP_FROM"),
		PublicURL:      publicURL,
		CORSOrigin:     corsOrigin,
		DesktopOrigins: desktopOrigins,

		GracefulTimeout:  getEnvDuration("GRACEFUL_TIMEOUT", 20*time.Second),
		PATTouchInterval: getEnvDuration("PAT_TOUCH_INTERVAL", 5*time.Minute),
	}
}

// getEnvDuration reads a Go duration string ("20s", "5m"). An unparseable value
// falls back to the default rather than failing the boot — a typo in an
// operational knob shouldn't keep the server down.
func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	d, err := time.ParseDuration(strings.TrimSpace(v))
	if err != nil || d < 0 {
		log.Printf("WARNING: %s=%q is not a valid duration — using %s", key, v, fallback)
		return fallback
	}
	return d
}

// splitCSV splits a comma-separated env value into a trimmed, non-empty slice.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
