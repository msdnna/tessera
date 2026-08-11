package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// #2684: the web SPA asks for its refresh token as an httpOnly cookie
// (X-Auth-Mode: cookie) so injected script can't read it, while Android, the
// desktop app and scripts keep getting it in the response body. These tests pin
// both halves of that contract — the new mode, and the absence of any change for
// the existing clients.

// authReq is doReq plus headers and cookies, which the shared helper doesn't
// carry. Redirects are not followed and no cookie jar is used on purpose: every
// test states exactly which cookie it presents.
func authReq(t *testing.T, method, path string, body any, headers map[string]string, cookies ...*http.Cookie) resp {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, testServer.URL+"/api"+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp{Status: res.StatusCode, Body: data, Header: res.Header}
}

// cookieMode is the header that switches refresh-token delivery to the cookie.
var cookieMode = map[string]string{"X-Auth-Mode": "cookie"}

// refreshCookie returns the tessera_refresh cookie from a response, or nil.
func refreshCookie(r resp) *http.Cookie {
	for _, ck := range (&http.Response{Header: r.Header}).Cookies() {
		if ck.Name == "tessera_refresh" {
			return ck
		}
	}
	return nil
}

// registerCookieMode signs up a fresh user in cookie mode and returns the
// response plus the cookie it set.
func registerCookieMode(t *testing.T) (resp, *http.Cookie) {
	t.Helper()
	n := userSeq.Add(1)
	email := fmt.Sprintf("cookie-user-%d@test.local", n)
	r := authReq(t, http.MethodPost, "/auth/register", map[string]any{
		"email": email, "name": "Cookie User", "password": "password-123",
	}, cookieMode)
	if r.Status != http.StatusOK && r.Status != http.StatusCreated {
		t.Fatalf("register: status %d\n%s", r.Status, r.Body)
	}
	ck := refreshCookie(r)
	if ck == nil {
		t.Fatalf("cookie-mode register set no tessera_refresh cookie\nSet-Cookie: %v", r.Header.Values("Set-Cookie"))
	}
	return r, ck
}

// The cookie carries the attributes that make it useful: unreadable from JS,
// not sent cross-site, and scoped to the auth routes rather than every call.
func TestRefreshCookieAttributes(t *testing.T) {
	t.Parallel()
	r, ck := registerCookieMode(t)

	if !ck.HttpOnly {
		t.Error("refresh cookie is not HttpOnly — script can read it, which is the whole point of the change")
	}
	if ck.SameSite != http.SameSiteStrictMode {
		t.Errorf("refresh cookie SameSite = %v, want Strict", ck.SameSite)
	}
	if ck.Path != "/api/auth" {
		t.Errorf("refresh cookie Path = %q, want /api/auth", ck.Path)
	}
	if ck.MaxAge < 29*24*3600 {
		t.Errorf("refresh cookie MaxAge = %d, want the full 30-day refresh TTL", ck.MaxAge)
	}
	// The harness PUBLIC_URL is http://, so Secure must be off — setting it
	// would make the browser drop the cookie silently on a plain-http dev
	// server. The https case is covered in handlers/auth_cookie_test.go.
	if ck.Secure {
		t.Error("refresh cookie is Secure behind an http PUBLIC_URL — the browser would discard it")
	}

	// And the token is gone from the body: that is what keeps it out of reach
	// of any script running on the page.
	var out map[string]any
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, r.Body)
	}
	if out["refresh_token"] != nil && out["refresh_token"] != "" {
		t.Errorf("cookie mode still returned refresh_token in the body: %v", out["refresh_token"])
	}
	if out["access_token"] == "" {
		t.Error("cookie mode returned no access token")
	}
}

// Refresh works on the cookie alone (empty body), rotates it, and kills the old
// token — the flow the SPA performs after every reload.
func TestRefreshWithCookieOnly(t *testing.T) {
	t.Parallel()
	_, ck := registerCookieMode(t)

	r := authReq(t, http.MethodPost, "/auth/refresh", map[string]any{}, cookieMode, ck)
	if r.Status != http.StatusOK {
		t.Fatalf("refresh by cookie: status %d\n%s", r.Status, r.Body)
	}
	rotated := refreshCookie(r)
	if rotated == nil || rotated.Value == "" {
		t.Fatal("refresh did not re-set the cookie — the next reload would use a revoked token")
	}
	if rotated.Value == ck.Value {
		t.Fatal("refresh returned the same token: rotation did not happen")
	}
	if m := r.mapBody(t); m["access_token"] == "" {
		t.Fatal("refresh returned no access token")
	}

	// The rotated-out token is dead, and the rejection clears the cookie so the
	// browser stops presenting it (JS cannot delete it itself).
	reuse := authReq(t, http.MethodPost, "/auth/refresh", map[string]any{}, cookieMode, ck)
	if reuse.Status != http.StatusUnauthorized {
		t.Fatalf("reusing a rotated cookie: status %d, want 401", reuse.Status)
	}
	if cleared := refreshCookie(reuse); cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("rejected cookie was not expired: %v", cleared)
	}
}

// A stale body value must not be able to steer a cookie session.
func TestRefreshCookieBeatsBody(t *testing.T) {
	t.Parallel()
	_, ck := registerCookieMode(t)

	r := authReq(t, http.MethodPost, "/auth/refresh",
		map[string]any{"refresh_token": "not-a-real-token"}, cookieMode, ck)
	if r.Status != http.StatusOK {
		t.Fatalf("cookie + junk body: status %d, want the cookie to win\n%s", r.Status, r.Body)
	}
}

// Regression guard for Android/desktop/scripts: without the header nothing
// changes — token in the body, no cookie in sight.
func TestRefreshBodyModeUnchanged(t *testing.T) {
	t.Parallel()
	c := signup(t)

	if c.Refresh == "" {
		t.Fatal("register without the header returned no refresh_token in the body")
	}
	reg := authReq(t, http.MethodPost, "/auth/login",
		map[string]any{"email": c.Email, "password": "password-123"}, nil)
	if ck := refreshCookie(reg); ck != nil {
		t.Errorf("a body-mode login set a refresh cookie (%v) — OkHttp would not store it and it only muddies the wire", ck)
	}

	r := doReq(t, "", http.MethodPost, "/auth/refresh", map[string]any{"refresh_token": c.Refresh})
	m := c.expect(t, r, http.StatusOK)
	if m["refresh_token"] == "" || m["refresh_token"] == c.Refresh {
		t.Fatalf("body-mode refresh did not rotate: %v", m)
	}
	if ck := refreshCookie(r); ck != nil {
		t.Errorf("body-mode refresh set a cookie: %v", ck)
	}
}

// A refresh with neither cookie nor body token is unauthorized, not a 400 —
// there is nothing malformed about "no session", and the SPA treats 401 as
// "show the login page".
func TestRefreshWithoutTokenIsUnauthorized(t *testing.T) {
	t.Parallel()
	for _, body := range []any{map[string]any{}, nil} {
		r := authReq(t, http.MethodPost, "/auth/refresh", body, cookieMode)
		if r.Status != http.StatusUnauthorized {
			t.Fatalf("refresh with body %v: status %d, want 401\n%s", body, r.Status, r.Body)
		}
	}
}

// Logout ends the session for real: the cookie is expired and the refresh token
// is revoked server-side. Before #2684 there was no logout endpoint at all and
// the row stayed usable for its full 30-day TTL.
func TestLogoutRevokesCookieSession(t *testing.T) {
	t.Parallel()
	_, ck := registerCookieMode(t)

	r := authReq(t, http.MethodPost, "/auth/logout", map[string]any{}, cookieMode, ck)
	if r.Status != http.StatusNoContent {
		t.Fatalf("logout: status %d\n%s", r.Status, r.Body)
	}
	cleared := refreshCookie(r)
	if cleared == nil || cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatalf("logout did not expire the cookie: %v", cleared)
	}

	after := authReq(t, http.MethodPost, "/auth/refresh", map[string]any{}, cookieMode, ck)
	if after.Status != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: status %d, want 401 — the token outlived the session\n%s",
			after.Status, after.Body)
	}
}

// Body-mode clients (Android, desktop) log out by posting the token, and an
// unknown token is still a successful, silent logout.
func TestLogoutBodyModeAndIdempotence(t *testing.T) {
	t.Parallel()
	c := signup(t)

	r := authReq(t, http.MethodPost, "/auth/logout", map[string]any{"refresh_token": c.Refresh}, nil)
	if r.Status != http.StatusNoContent {
		t.Fatalf("body-mode logout: status %d\n%s", r.Status, r.Body)
	}
	after := doReq(t, "", http.MethodPost, "/auth/refresh", map[string]any{"refresh_token": c.Refresh})
	if after.Status != http.StatusUnauthorized {
		t.Fatalf("refresh after body-mode logout: status %d, want 401", after.Status)
	}

	// Repeat, plus a token that never existed: both answer 204. A 404 here
	// would let an attacker probe which stolen tokens are still live.
	for _, tok := range []string{c.Refresh, "never-issued-token"} {
		again := authReq(t, http.MethodPost, "/auth/logout", map[string]any{"refresh_token": tok}, nil)
		if again.Status != http.StatusNoContent {
			t.Fatalf("logout with %q: status %d, want 204", tok, again.Status)
		}
	}
	// Nothing at all to revoke is also fine.
	empty := authReq(t, http.MethodPost, "/auth/logout", nil, cookieMode)
	if empty.Status != http.StatusNoContent {
		t.Fatalf("logout without a token: status %d, want 204", empty.Status)
	}
}
