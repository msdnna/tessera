// Package config loads and validates runtime configuration from the environment.
package config

import (
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"strconv"
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
	// Path to a Firebase service-account key (JSON) used to send background
	// push to Android device channels over FCM HTTP v1. Optional and *not*
	// fail-closed in production: push is a best-effort transport, so a missing
	// or unreadable key downgrades device channels to the live-WS-only
	// behaviour they had before instead of keeping the server down.
	FCMCredentialsFile string
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
	// Reverse proxies whose X-Forwarded-For is trusted when deriving the client
	// IP. This is what the rate limiter keys on, so getting it wrong collapses
	// every client behind the proxy into a single bucket (or, the other way,
	// lets a client pick its own bucket). Defaults to loopback — override with
	// TRUSTED_PROXIES (comma-separated) when the proxy is a separate host.
	TrustedProxies []string
	// Throttle the unauthenticated auth routes. On by default; RATE_LIMIT_ENABLED=false
	// turns it off (single-user installs behind a private network).
	RateLimitEnabled bool
	// Require a credential (media cookie or bearer token) on /api/uploads/:name.
	// Off by default: the desktop app loads inline images as ordinary cross-site
	// <img> tags, which carry no host-only cookie, so an install that has desktop
	// users would lose every picture. Web and Android work either way — turn
	// MEDIA_REQUIRE_AUTH=true on to stop serving uploads to anonymous callers.
	MediaRequireAuth bool
	// Base URL of the LibreOffice conversion sidecar (converter/), e.g.
	// http://converter:3000. Empty disables office import/export: the Documents
	// section keeps working in full and only those two actions report themselves
	// as unavailable. Deliberately empty by default — the sidecar is close to a
	// gigabyte, and an install that does not want it should not have to opt out.
	ConverterURL string
	// Request body ceilings, in bytes. MaxBodyBytes is the blanket limit;
	// uploads and attachments get their own, larger, budgets.
	MaxBodyBytes       int64
	MaxUploadBytes     int64
	MaxAttachmentBytes int64
	// Optional error/performance telemetry (Sentry). All empty by default:
	// a blank SentryDSN leaves the SDK uninitialised and every capture path a
	// no-op, and a blank SentryFrontendDSN tells the browser not to load its
	// own SDK at all. The two DSNs are separate because a Sentry project is
	// per-platform — backend events belong to a Go project, browser events to a
	// JavaScript one, and they carry different sample rates.
	SentryDSN                string
	SentryEnv                string
	SentryTracesRate         float64
	SentryFrontendDSN        string
	SentryFrontendTracesRate float64
}

// Body-size defaults, also used as the fallback when a Config is built
// programmatically (tests) and leaves the fields zero.
//
// These are *transport* ceilings on the whole request, and they sit
// deliberately above the per-file caps the upload handlers enforce (8 MiB
// inline media, 25 MiB attachments): a multipart request also carries part
// headers, boundaries and form fields, so a file right at its cap would
// otherwise be cut off by the transport before the handler could give its own
// answer. The slack is framing overhead, not extra payload allowance.
const (
	DefaultMaxBodyBytes       int64 = 1 << 20  // 1 MiB — JSON payloads
	DefaultMaxUploadBytes     int64 = 9 << 20  // 8 MiB media + framing
	DefaultMaxAttachmentBytes int64 = 26 << 20 // 25 MiB attachment + framing
)

// fatal reports a fail-closed misconfiguration and stops the process.
//
// Deliberately not log.Fatal: main() calls slog.SetDefault before New(), and
// that routes the standard log package through the slog handler at INFO level —
// so on a box running LOG_LEVEL=warn (an ordinary production setting) the guards
// below would kill the process without printing a word, leaving an operator with
// an exit code and no reason. slog.Error clears every threshold the logger
// accepts. Caught by the e2e boot suite (#2709), which asserts the message is
// still there under LOG_LEVEL=warn.
func fatal(msg string) {
	slog.Error(msg)
	os.Exit(1)
}

// New reads configuration from the environment. In production
// (APP_ENV=production) JWT_SECRET and DATABASE_URL must be explicitly set —
// fail-closed prevents the binary from booting with dev defaults.
func New() *Config {
	prod := strings.EqualFold(os.Getenv("APP_ENV"), "production")

	jwt := os.Getenv("JWT_SECRET")
	if jwt == "" {
		if prod {
			fatal("JWT_SECRET is required in production (APP_ENV=production)")
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
			fatal("DATABASE_URL is required in production (APP_ENV=production)")
		}
		dbURL = "postgres://tessera:tessera@localhost:5432/tessera?sslmode=disable"
		log.Println("WARNING: DATABASE_URL not set — using local dev default")
	}

	// Key for encrypting secrets at rest (GitLab PATs). Independent from
	// JWT_SECRET so rotating one doesn't invalidate the other.
	encKey := os.Getenv("ENCRYPTION_KEY")
	if encKey == "" {
		if prod {
			fatal("ENCRYPTION_KEY is required in production (APP_ENV=production)")
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

	// Sentry's environment tag falls back to APP_ENV so a deployment names its
	// environment once; SENTRY_ENV exists only to override it. Blank counts as
	// unset here rather than as an answer — compose passes the whole optional
	// block through as SENTRY_ENV=${SENTRY_ENV:-}, so in a container the variable
	// is present-but-empty whenever the operator hasn't set it, and a plain
	// LookupEnv fallback would tag every production event with "".
	sentryEnv := strings.TrimSpace(os.Getenv("SENTRY_ENV"))
	if sentryEnv == "" {
		sentryEnv = strings.TrimSpace(os.Getenv("APP_ENV"))
	}
	if sentryEnv == "" {
		sentryEnv = "development"
	}

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

		FCMCredentialsFile: strings.TrimSpace(os.Getenv("FCM_CREDENTIALS_FILE")),

		GracefulTimeout:    getEnvDuration("GRACEFUL_TIMEOUT", 20*time.Second),
		PATTouchInterval:   getEnvDuration("PAT_TOUCH_INTERVAL", 5*time.Minute),
		TrustedProxies:     splitCSV(getEnv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		RateLimitEnabled:   getEnvBool("RATE_LIMIT_ENABLED", true),
		MediaRequireAuth:   getEnvBool("MEDIA_REQUIRE_AUTH", false),
		ConverterURL:       getEnv("CONVERTER_URL", ""),
		MaxBodyBytes:       getEnvBytes("MAX_BODY_BYTES", DefaultMaxBodyBytes),
		MaxUploadBytes:     getEnvBytes("MAX_UPLOAD_BYTES", DefaultMaxUploadBytes),
		MaxAttachmentBytes: getEnvBytes("MAX_ATTACHMENT_BYTES", DefaultMaxAttachmentBytes),

		SentryDSN: strings.TrimSpace(os.Getenv("SENTRY_DSN")),
		SentryEnv: sentryEnv,
		// Server transactions are cheap and low-volume for a self-hosted
		// tracker, so sample all of them; browser transactions include every
		// page load and route change, hence the far lower default.
		SentryTracesRate:         getEnvRate("SENTRY_TRACES_SAMPLE_RATE", 1.0),
		SentryFrontendDSN:        strings.TrimSpace(os.Getenv("SENTRY_FRONTEND_DSN")),
		SentryFrontendTracesRate: getEnvRate("SENTRY_FRONTEND_TRACES_SAMPLE_RATE", 0.1),
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

// getEnvBool parses a boolean env var, keeping the fallback on anything it
// can't read — a typo in a security knob must not silently turn it off.
func getEnvBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		log.Printf("WARNING: %s=%q is not a boolean — keeping %v", key, v, fallback)
		return fallback
	}
	return b
}

// getEnvRate parses a sample rate and clamps it to [0,1] — the range the Sentry
// SDK accepts. An unreadable value keeps the default rather than failing the
// boot: a typo in a telemetry knob must not cost the operator their API.
func getEnvRate(key string, fallback float64) float64 {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		log.Printf("WARNING: %s=%q is not a number — keeping %.2f", key, v, fallback)
		return fallback
	}
	switch {
	case f < 0:
		log.Printf("WARNING: %s=%q is below 0 — clamping to 0 (tracing off)", key, v)
		return 0
	case f > 1:
		log.Printf("WARNING: %s=%q is above 1 — clamping to 1 (sample everything)", key, v)
		return 1
	}
	return f
}

// getEnvBytes parses a byte count, rejecting non-positive values (a zero limit
// would reject every request with a body).
func getEnvBytes(key string, fallback int64) int64 {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n <= 0 {
		log.Printf("WARNING: %s=%q is not a positive byte count — keeping %d", key, v, fallback)
		return fallback
	}
	return n
}
