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
		"SENTRY_DSN", "SENTRY_ENV", "SENTRY_TRACES_SAMPLE_RATE",
		"SENTRY_FRONTEND_DSN", "SENTRY_FRONTEND_TRACES_SAMPLE_RATE",
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

// Telemetry must be off unless the operator opts in, and must name the
// environment without being told twice.
func TestSentryDefaultsOff(t *testing.T) {
	clearEnv(t)
	cfg := New()

	if cfg.SentryDSN != "" || cfg.SentryFrontendDSN != "" {
		t.Errorf("Sentry DSNs should be empty by default: %q / %q", cfg.SentryDSN, cfg.SentryFrontendDSN)
	}
	if cfg.SentryEnv != "development" {
		t.Errorf("SentryEnv = %q, want development", cfg.SentryEnv)
	}
	if cfg.SentryTracesRate != 1.0 {
		t.Errorf("SentryTracesRate = %v, want 1.0", cfg.SentryTracesRate)
	}
	if cfg.SentryFrontendTracesRate != 0.1 {
		t.Errorf("SentryFrontendTracesRate = %v, want 0.1", cfg.SentryFrontendTracesRate)
	}
}

// SENTRY_ENV defaults to APP_ENV so a deployment names its environment once.
func TestSentryEnvFollowsAppEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "staging")
	if got := New().SentryEnv; got != "staging" {
		t.Errorf("SentryEnv = %q, want staging (inherited from APP_ENV)", got)
	}

	t.Setenv("SENTRY_ENV", "canary")
	if got := New().SentryEnv; got != "canary" {
		t.Errorf("explicit SENTRY_ENV ignored: %q", got)
	}
}

// docker-compose passes the optional block through as SENTRY_ENV=${SENTRY_ENV:-},
// so in a container the variable is *present and empty* whenever the operator
// left it alone. Present-and-empty must mean "unset", not "the environment is
// called empty-string" — otherwise every production event lands untagged.
func TestSentryEnvBlankFallsBackToAppEnv(t *testing.T) {
	clearEnv(t)
	// "staging" rather than "production" — the latter trips the fail-closed
	// secret guards, which is a different test's business.
	t.Setenv("APP_ENV", "staging")
	t.Setenv("SENTRY_ENV", "")

	if got := New().SentryEnv; got != "staging" {
		t.Errorf("SentryEnv = %q, want staging (blank SENTRY_ENV must not win)", got)
	}

	// Both blank → the same default a bare dev checkout gets.
	t.Setenv("APP_ENV", "")
	if got := New().SentryEnv; got != "development" {
		t.Errorf("SentryEnv = %q, want development", got)
	}
}

func TestSentryRateClampAndFallback(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want float64
	}{
		{"0.25", 0.25},
		{"0", 0},
		{"5", 1},    // above the SDK's range → sample everything
		{"-2", 0},   // below → tracing off
		{"lots", 1}, // unreadable → keep the default rather than fail the boot
		{"", 1},     // explicitly blank → default
	} {
		clearEnv(t)
		t.Setenv("SENTRY_TRACES_SAMPLE_RATE", tc.raw)
		if got := New().SentryTracesRate; got != tc.want {
			t.Errorf("SENTRY_TRACES_SAMPLE_RATE=%q → %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// A DSN pasted with a stray newline or space must still work — this is a value
// operators copy out of a web UI.
func TestSentryDSNTrimmed(t *testing.T) {
	clearEnv(t)
	t.Setenv("SENTRY_DSN", "  http://key@host/1\n")
	if got := New().SentryDSN; got != "http://key@host/1" {
		t.Errorf("SentryDSN = %q, want it trimmed", got)
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
