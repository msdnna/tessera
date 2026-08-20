//go:build e2e

// The refresh cookie against a real process. auth_cookie_flow_test.go covers the
// same endpoints in-process, but on httptest — where the scheme is always plain
// http and the process never restarts. The two things that can only be seen from
// out here: the Secure attribute the SPA's browser will actually receive (the
// "works on localhost, silently drops the cookie behind nginx" class of bug),
// and whether a session survives a redeploy.
package e2e

import (
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

const refreshCookie = "tessera_refresh"

func cookieClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	// Keyed by host, not port — the same jar follows the session across the
	// restart below, exactly as a browser would.
	return &http.Client{Jar: jar, Timeout: 20 * time.Second}
}

func findCookie(r resp, name string) *http.Cookie {
	for _, c := range (&http.Response{Header: r.Header}).Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// TestRefreshCookieSurvivesRestart is the deploy scenario: users are logged in,
// the process is replaced, nobody should be logged out. It fails if JWT_SECRET
// is ever generated at boot instead of read from the environment.
func TestRefreshCookieSurvivesRestart(t *testing.T) {
	client := cookieClient(t)
	old := startServer(t, nil)

	reg := old.call(t, request{
		Method: http.MethodPost, Path: "/auth/register", Client: client,
		Header: http.Header{"X-Auth-Mode": []string{"cookie"}},
		Body: map[string]any{
			"email": "e2e-cookie-" + runID + "@test.local", "name": "E2E Cookie", "password": "password-123",
		},
	})
	body := expect(t, reg, http.StatusOK)
	if _, present := body["refresh_token"]; present {
		t.Error("cookie-mode registration leaked the refresh token into the JSON body")
	}
	ck := findCookie(reg, refreshCookie)
	if ck == nil {
		t.Fatalf("no %s cookie in the response headers: %v", refreshCookie, reg.Header)
	}
	if !ck.HttpOnly {
		t.Error("refresh cookie is readable by JavaScript (HttpOnly missing)")
	}
	if ck.Path != "/api/auth" {
		t.Errorf("refresh cookie path = %q, want /api/auth", ck.Path)
	}
	if ck.SameSite != http.SameSiteStrictMode {
		t.Errorf("refresh cookie SameSite = %v, want Strict", ck.SameSite)
	}
	if ck.Secure {
		t.Error("refresh cookie is Secure over plain http — the browser would drop it on a dev/LAN install")
	}

	// Replace the process, same secret and same database: a deploy.
	old.stop()
	fresh := startServer(t, nil)

	// Empty body on purpose — the cookie is the only credential presented.
	rot := fresh.call(t, request{
		Method: http.MethodPost, Path: "/auth/refresh", Client: client,
		Body: map[string]any{},
	})
	rotated := expect(t, rot, http.StatusOK)
	if _, present := rotated["refresh_token"]; present {
		t.Error("rotation leaked the new refresh token into the body of a cookie-mode client")
	}
	access, _ := rotated["access_token"].(string)
	if access == "" {
		t.Fatalf("rotation returned no access token: %s", rot.Body)
	}
	me := expect(t, fresh.get(t, "/auth/me", access), http.StatusOK)
	if user, _ := me["user"].(map[string]any); user == nil || user["email"] == "" {
		t.Errorf("/auth/me after restart = %v", me)
	}
}

// TestRefreshGraceSurvivesLostResponse is the reconnect scenario from #2750,
// end to end: rotation revokes the presented token before the new pair is on the
// wire, so a connection that drops in between leaves the client holding a dead
// token and the next attempt used to land on the login screen. Presenting the
// old cookie again inside the grace window must hand back a working pair — and
// an explicit logout must still take effect immediately.
func TestRefreshGraceSurvivesLostResponse(t *testing.T) {
	srv := startServer(t, nil)
	// No cookie jar: every call below states exactly which cookie it presents,
	// including the one a browser would still be holding after a lost response.
	client := &http.Client{Timeout: 20 * time.Second}
	present := func(c *http.Cookie) http.Header {
		return http.Header{
			"X-Auth-Mode": []string{"cookie"},
			"Cookie":      []string{c.Name + "=" + c.Value},
		}
	}

	reg := srv.call(t, request{
		Method: http.MethodPost, Path: "/auth/register", Client: client,
		Header: http.Header{"X-Auth-Mode": []string{"cookie"}},
		Body: map[string]any{
			"email": "e2e-grace-" + runID + "@test.local", "name": "E2E Grace", "password": "password-123",
		},
	})
	expect(t, reg, http.StatusOK)
	first := findCookie(reg, refreshCookie)
	if first == nil {
		t.Fatalf("no %s cookie after registration: %v", refreshCookie, reg.Header)
	}

	// The rotation whose response we are about to pretend never arrived.
	rot := srv.call(t, request{
		Method: http.MethodPost, Path: "/auth/refresh", Client: client,
		Header: present(first), Body: map[string]any{},
	})
	expect(t, rot, http.StatusOK)
	second := findCookie(rot, refreshCookie)
	if second == nil || second.Value == first.Value {
		t.Fatalf("rotation did not issue a new cookie: %v", second)
	}

	// The client never saw that response, so it retries with the old cookie.
	replay := srv.call(t, request{
		Method: http.MethodPost, Path: "/auth/refresh", Client: client,
		Header: present(first), Body: map[string]any{},
	})
	body := expect(t, replay, http.StatusOK)
	access, _ := body["access_token"].(string)
	if access == "" {
		t.Fatalf("grace replay returned no access token: %s", replay.Body)
	}
	if regrant := findCookie(replay, refreshCookie); regrant == nil || regrant.Value == "" {
		t.Fatal("grace replay handed back no usable refresh cookie")
	}
	// The recovered session is real, not just a 200.
	expect(t, srv.get(t, "/auth/me", access), http.StatusOK)

	// Signing out is immediate — the grace window must not keep a deliberately
	// ended session alive for another half-minute.
	out := srv.call(t, request{
		Method: http.MethodPost, Path: "/auth/logout", Client: client,
		Header: present(second), Body: map[string]any{},
	})
	expect(t, out, http.StatusNoContent)
	after := srv.call(t, request{
		Method: http.MethodPost, Path: "/auth/refresh", Client: client,
		Header: present(second), Body: map[string]any{},
	})
	if after.Status != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: status %d, want 401\n%s", after.Status, after.Body)
	}
}

// TestRefreshCookieIsSecureBehindTLS covers the nginx case: the request reaching
// the process is plain http, so PUBLIC_URL is the only signal that the browser
// spoke https. Get it wrong and the cookie goes out without Secure.
func TestRefreshCookieIsSecureBehindTLS(t *testing.T) {
	s := startServer(t, map[string]string{"PUBLIC_URL": "https://tessera.example"})
	reg := s.call(t, request{
		Method: http.MethodPost, Path: "/auth/register", Client: cookieClient(t),
		Header: http.Header{"X-Auth-Mode": []string{"cookie"}},
		Body: map[string]any{
			"email": "e2e-secure-" + runID + "@test.local", "name": "E2E Secure", "password": "password-123",
		},
	})
	expect(t, reg, http.StatusOK)
	ck := findCookie(reg, refreshCookie)
	if ck == nil {
		t.Fatalf("no %s cookie: %v", refreshCookie, reg.Header)
	}
	if !ck.Secure {
		t.Error("refresh cookie lacks Secure although PUBLIC_URL is https — it would ride a plain-http hop")
	}
}
