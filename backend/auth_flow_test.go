package main

import (
	"net/http"
	"testing"
	"time"
)

// Smoke: register → me → refresh → login round-trip.
func TestAuthRegisterLoginRefresh(t *testing.T) {
	t.Parallel()
	c := signup(t)

	me := c.expect(t, c.get("/auth/me"), http.StatusOK)
	user, ok := me["user"].(map[string]any)
	if !ok {
		// Some shapes return the user at top level.
		user = me
	}
	if user["email"] != c.Email {
		t.Fatalf("me email = %v, want %s\n%s", user["email"], c.Email, me)
	}

	// Refresh rotates the pair.
	r := doReq(t, "", http.MethodPost, "/auth/refresh", map[string]any{"refresh_token": c.Refresh})
	m := c.expect(t, r, http.StatusOK)
	if m["access_token"] == "" || m["refresh_token"] == c.Refresh {
		t.Fatalf("refresh did not rotate: %v", m)
	}

	// Login with the same credentials.
	r = doReq(t, "", http.MethodPost, "/auth/login", map[string]any{"email": c.Email, "password": "password-123"})
	c.expect(t, r, http.StatusOK)

	// Wrong password → 401.
	r = doReq(t, "", http.MethodPost, "/auth/login", map[string]any{"email": c.Email, "password": "wrong-password"})
	if r.Status != http.StatusUnauthorized {
		t.Fatalf("wrong password: status %d", r.Status)
	}

	// Duplicate email → 409.
	r = doReq(t, "", http.MethodPost, "/auth/register", map[string]any{"email": c.Email, "name": "x", "password": "password-123"})
	if r.Status != http.StatusConflict {
		t.Fatalf("duplicate register: status %d", r.Status)
	}
}

// #2626: a login against an unknown email must be indistinguishable from a
// login with the wrong password — same status, same body, and a comparable
// amount of work. Before the fix the unknown-email branch skipped bcrypt
// entirely and answered ~1000x faster, which enumerates accounts with nothing
// but a stopwatch.
func TestLoginDoesNotEnumerateEmails(t *testing.T) {
	t.Parallel()
	c := signup(t)

	start := time.Now()
	known := doReq(t, "", http.MethodPost, "/auth/login",
		map[string]any{"email": c.Email, "password": "wrong-password"})
	knownDur := time.Since(start)

	start = time.Now()
	unknown := doReq(t, "", http.MethodPost, "/auth/login",
		map[string]any{"email": "no-such-" + c.Email, "password": "wrong-password"})
	unknownDur := time.Since(start)

	if known.Status != http.StatusUnauthorized || unknown.Status != http.StatusUnauthorized {
		t.Fatalf("statuses: known-email %d, unknown-email %d, want both 401", known.Status, unknown.Status)
	}
	if string(known.Body) != string(unknown.Body) {
		t.Fatalf("responses differ and leak whether the account exists:\nknown:   %s\nunknown: %s", known.Body, unknown.Body)
	}
	// Deliberately loose: this asserts "bcrypt ran at all", not equal timing.
	// The regression it guards against is three orders of magnitude, while a
	// tight bound would just be flaky under -race and parallel tests.
	if unknownDur*4 < knownDur {
		t.Fatalf("unknown-email login took %v vs %v for a known email — bcrypt looks skipped", unknownDur, knownDur)
	}
}

// The fixture chain works and a new board is seeded with default columns.
func TestFixtureStack(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	if len(s.Columns) != 4 {
		t.Fatalf("new board has %d columns, want 4", len(s.Columns))
	}
	task := mkTask(t, c, s.Board, s.col(t, 0), "Первая задача")
	if task["title"] != "Первая задача" {
		t.Fatalf("task create: %v", task)
	}
}
