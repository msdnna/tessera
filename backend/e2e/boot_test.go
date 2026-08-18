//go:build e2e

// Boot-time behaviour: what main() and config.New() do with a given environment.
// None of this is reachable from the in-process harness — it builds a
// config.Config literal in Go and never runs main() at all, and the production
// guards it would have to trip call log.Fatal, which takes the test binary down
// with them.
package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBootServesHealthAndReady(t *testing.T) {
	s := startServer(t, nil)

	health := expect(t, s.get(t, "/health", ""), http.StatusOK)
	if health["ok"] != true || health["app"] != "tessera" {
		t.Fatalf("/health = %v, want ok=true app=tessera", health)
	}

	ready := expect(t, s.get(t, "/health/ready", ""), http.StatusOK)
	if ready["ok"] != true {
		t.Fatalf("/health/ready = %v, want ok=true", ready)
	}
	db, _ := ready["db"].(map[string]any)
	if db == nil || db["ok"] != true {
		t.Fatalf("/health/ready db = %v, want ok=true — the pool from main() is not usable", ready["db"])
	}
	// Readiness carries the build version; an empty one means the ldflags/embed
	// wiring in main broke, which no unit test would notice.
	if v, _ := ready["version"].(string); v == "" {
		t.Errorf("/health/ready reported an empty version")
	}
}

// TestProductionGuardsFailClosed is the reason this suite exists. config.New
// answers a missing production secret with log.Fatal — in-process that would
// kill the test runner, so the only way to assert it is to watch a subprocess
// die. Without this, "forgot JWT_SECRET in prod" ships green and fails on the box.
func TestProductionGuardsFailClosed(t *testing.T) {
	cases := []struct {
		name    string
		drop    string // env var withheld
		wantLog string
	}{
		{"jwt secret", "JWT_SECRET", "JWT_SECRET is required in production"},
		{"database url", "DATABASE_URL", "DATABASE_URL is required in production"},
		{"encryption key", "ENCRYPTION_KEY", "ENCRYPTION_KEY is required in production"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := serverEnv(freePort(t))
			env["APP_ENV"] = "production"
			delete(env, tc.drop)

			s := launch(t, env)
			code, exited := s.awaitExit(30 * time.Second)
			if !exited {
				t.Fatalf("server kept running in production without %s — fail-closed is broken\n%s",
					tc.drop, s.stderr.String())
			}
			if code == 0 {
				t.Errorf("exit code 0, want non-zero")
			}
			if out := s.stderr.String(); !strings.Contains(out, tc.wantLog) {
				t.Errorf("output does not explain the failure (want %q):\n%s", tc.wantLog, out)
			}
		})
	}
}

// TestProductionGuardStaysVisibleAtWarnLevel guards the fix this suite forced
// (#2709): main() installs the slog handler before config.New runs, and
// slog.SetDefault routes the standard log package through it at INFO — so while
// the guard used log.Fatal, a production box with LOG_LEVEL=warn died silently,
// handing its operator an exit code and no reason at all.
func TestProductionGuardStaysVisibleAtWarnLevel(t *testing.T) {
	env := serverEnv(freePort(t))
	env["APP_ENV"] = "production"
	env["LOG_LEVEL"] = "warn"
	delete(env, "JWT_SECRET")

	s := launch(t, env)
	if _, exited := s.awaitExit(30 * time.Second); !exited {
		t.Fatalf("server kept running without JWT_SECRET in production\n%s", s.stderr.String())
	}
	if out := s.stderr.String(); !strings.Contains(out, "JWT_SECRET is required in production") {
		t.Errorf("the process died silently at LOG_LEVEL=warn — nothing explains why:\n%s", out)
	}
}

// TestProductionBootsWithFullEnv is the other half of the guard: a complete
// production environment must actually serve, not just refuse to start.
func TestProductionBootsWithFullEnv(t *testing.T) {
	s := startServer(t, map[string]string{
		"APP_ENV":    "production",
		"PUBLIC_URL": "https://tessera.example",
	})
	if h := expect(t, s.get(t, "/health", ""), http.StatusOK); h["ok"] != true {
		t.Fatalf("/health = %v in production mode", h)
	}
}

// TestCORSOriginByEnv pins the "locked in prod, open in dev" rule to the real
// process reading real env vars — the place where a wrong default actually costs
// something (the SPA silently blocked, or the API open to every origin).
func TestCORSOriginByEnv(t *testing.T) {
	const public = "https://tessera.example"

	t.Run("production locks to PUBLIC_URL", func(t *testing.T) {
		s := startServer(t, map[string]string{"APP_ENV": "production", "PUBLIC_URL": public})

		allowed := s.call(t, request{Method: http.MethodGet, Path: "/health",
			Header: http.Header{"Origin": []string{public}}})
		if got := allowed.Header.Get("Access-Control-Allow-Origin"); got != public {
			t.Errorf("Allow-Origin for the public URL = %q, want %q", got, public)
		}

		denied := s.call(t, request{Method: http.MethodGet, Path: "/health",
			Header: http.Header{"Origin": []string{"https://evil.example"}}})
		if got := denied.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Allow-Origin for a foreign origin = %q, want it absent", got)
		}
	})

	t.Run("development allows any origin", func(t *testing.T) {
		s := startServer(t, nil)
		r := s.call(t, request{Method: http.MethodGet, Path: "/health",
			Header: http.Header{"Origin": []string{"https://evil.example"}}})
		if got := r.Header.Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Allow-Origin in dev = %q, want *", got)
		}
	})
}
