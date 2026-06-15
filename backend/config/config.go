package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
)

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

	return &Config{
		DatabaseURL:   dbURL,
		Port:          getEnv("PORT", "8080"),
		JWTSecret:     jwt,
		AppEnv:        getEnv("APP_ENV", "development"),
		UploadDir:     getEnv("UPLOAD_DIR", "./uploads"),
		EncryptionKey: encKey,
		SMTPHost:      os.Getenv("SMTP_HOST"),
		SMTPPort:      getEnv("SMTP_PORT", "587"),
		SMTPUser:      os.Getenv("SMTP_USER"),
		SMTPPass:      os.Getenv("SMTP_PASS"),
		SMTPFrom:      os.Getenv("SMTP_FROM"),
		PublicURL:     os.Getenv("PUBLIC_URL"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}
