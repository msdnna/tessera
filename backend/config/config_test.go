package config

import (
	"os"
	"strings"
	"testing"
)

// clearEnv unsets every variable config.New reads so host values don't leak
// in. t.Setenv first registers the restore-on-cleanup, then the actual unset
// removes the key (getEnv uses LookupEnv, so presence matters).
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"APP_ENV", "JWT_SECRET", "DATABASE_URL", "POSTGRES_USER", "POSTGRES_PASSWORD",
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB", "ENCRYPTION_KEY", "PUBLIC_URL",
		"CORS_ORIGIN", "DESKTOP_CORS_ORIGINS", "PORT", "UPLOAD_DIR",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASS", "SMTP_FROM",
		"TRUSTED_PROXIES", "RATE_LIMIT_ENABLED", "MEDIA_REQUIRE_AUTH",
		"MAX_BODY_BYTES", "MAX_UPLOAD_BYTES", "MAX_ATTACHMENT_BYTES",
	} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
}

func TestNewDevDefaults(t *testing.T) {
	clearEnv(t)
	cfg := New()
	if cfg.Port != "8080" || cfg.AppEnv == "" {
		t.Fatalf("defaults: %+v", cfg)
	}
	if !strings.Contains(cfg.DatabaseURL, "localhost:5432/tessera") {
		t.Fatalf("db url default: %s", cfg.DatabaseURL)
	}
	if cfg.JWTSecret == "" || cfg.EncryptionKey == "" {
		t.Fatal("dev secrets must fall back to non-empty defaults")
	}
	if cfg.CORSOrigin != "*" {
		t.Fatalf("dev CORS = %s, want *", cfg.CORSOrigin)
	}
	if len(cfg.DesktopOrigins) != 3 {
		t.Fatalf("desktop origins: %v", cfg.DesktopOrigins)
	}
}

func TestNewStitchesPostgresVars(t *testing.T) {
	clearEnv(t)
	t.Setenv("POSTGRES_USER", "u ser") // needs escaping
	t.Setenv("POSTGRES_PASSWORD", "p@ss")
	t.Setenv("POSTGRES_HOST", "dbhost")
	t.Setenv("POSTGRES_PORT", "5433")
	t.Setenv("POSTGRES_DB", "custom")
	cfg := New()
	want := "postgres://u+ser:p%40ss@dbhost:5433/custom?sslmode=disable"
	if cfg.DatabaseURL != want {
		t.Fatalf("stitched url = %s, want %s", cfg.DatabaseURL, want)
	}
}

func TestNewProdLocksCORSToPublicURL(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "prod-secret-prod-secret-prod-secret!")
	t.Setenv("DATABASE_URL", "postgres://x:y@db:5432/tessera")
	t.Setenv("ENCRYPTION_KEY", "prod-encryption-key")
	t.Setenv("PUBLIC_URL", "https://tessera.example.com")
	cfg := New()
	if cfg.CORSOrigin != "https://tessera.example.com" {
		t.Fatalf("prod CORS = %s", cfg.CORSOrigin)
	}
	// Explicit CORS_ORIGIN wins over the PublicURL fallback.
	t.Setenv("CORS_ORIGIN", "https://other.example.com")
	if got := New().CORSOrigin; got != "https://other.example.com" {
		t.Fatalf("explicit CORS = %s", got)
	}
}

func TestLimitDefaults(t *testing.T) {
	clearEnv(t)
	cfg := New()

	if !cfg.RateLimitEnabled {
		t.Error("auth throttling must be on unless explicitly disabled")
	}
	if cfg.MaxBodyBytes != DefaultMaxBodyBytes ||
		cfg.MaxUploadBytes != DefaultMaxUploadBytes ||
		cfg.MaxAttachmentBytes != DefaultMaxAttachmentBytes {
		t.Errorf("body limits: %d/%d/%d", cfg.MaxBodyBytes, cfg.MaxUploadBytes, cfg.MaxAttachmentBytes)
	}
	if len(cfg.TrustedProxies) != 2 {
		t.Errorf("trusted proxies default = %v, want the loopback pair", cfg.TrustedProxies)
	}
}

// Media stays open unless an operator says otherwise: the desktop client loads
// images as cross-site <img> tags that carry no cookie, so a default-on switch
// would blank them out on upgrade.
func TestMediaRequireAuthDefaultsOff(t *testing.T) {
	clearEnv(t)
	if New().MediaRequireAuth {
		t.Error("MEDIA_REQUIRE_AUTH defaulted to on")
	}
	t.Setenv("MEDIA_REQUIRE_AUTH", "true")
	if !New().MediaRequireAuth {
		t.Error("MEDIA_REQUIRE_AUTH=true ignored")
	}
}

func TestLimitOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("RATE_LIMIT_ENABLED", "false")
	t.Setenv("MAX_BODY_BYTES", "4096")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.1, 10.0.0.2")
	cfg := New()

	if cfg.RateLimitEnabled {
		t.Error("RATE_LIMIT_ENABLED=false ignored")
	}
	if cfg.MaxBodyBytes != 4096 {
		t.Errorf("MAX_BODY_BYTES = %d, want 4096", cfg.MaxBodyBytes)
	}
	if len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "10.0.0.1" {
		t.Errorf("TRUSTED_PROXIES = %v", cfg.TrustedProxies)
	}
}

// A malformed security knob must keep the safe default rather than resolve to
// the zero value — "off" and "no limit" are exactly the dangerous readings.
func TestLimitGarbageKeepsDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("RATE_LIMIT_ENABLED", "yes-please")
	t.Setenv("MAX_BODY_BYTES", "0")
	t.Setenv("MAX_UPLOAD_BYTES", "-1")
	t.Setenv("MAX_ATTACHMENT_BYTES", "twenty")
	cfg := New()

	if !cfg.RateLimitEnabled {
		t.Error("unparseable RATE_LIMIT_ENABLED turned throttling off")
	}
	if cfg.MaxBodyBytes != DefaultMaxBodyBytes {
		t.Errorf("MAX_BODY_BYTES=0 accepted: %d", cfg.MaxBodyBytes)
	}
	if cfg.MaxUploadBytes != DefaultMaxUploadBytes {
		t.Errorf("negative MAX_UPLOAD_BYTES accepted: %d", cfg.MaxUploadBytes)
	}
	if cfg.MaxAttachmentBytes != DefaultMaxAttachmentBytes {
		t.Errorf("unparseable MAX_ATTACHMENT_BYTES accepted: %d", cfg.MaxAttachmentBytes)
	}
}

func TestSplitCSV(t *testing.T) {
	if got := splitCSV(" a, ,b ,"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitCSV: %v", got)
	}
	if got := splitCSV(""); got != nil {
		t.Fatalf("splitCSV empty: %v", got)
	}
}
