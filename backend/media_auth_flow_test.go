package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"tessera/config"
	"tessera/internal/mail"
	"tessera/internal/realtime"
)

// #2685 (part B): inline images live behind a credential an <img> can actually
// carry — the httpOnly media cookie for the browser, a bearer token for Android.
// The shared harness leaves MEDIA_REQUIRE_AUTH off (the production default,
// which keeps the desktop client's cross-site <img> tags working), so the
// enforcing half of the contract gets a server of its own below.

// mediaCookie returns the tessera_media cookie from a response, or nil.
func mediaCookie(r resp) *http.Cookie {
	for _, ck := range (&http.Response{Header: r.Header}).Cookies() {
		if ck.Name == "tessera_media" {
			return ck
		}
	}
	return nil
}

// The image credential ships with the session that minted it, scoped so it can
// unlock nothing but the media route.
func TestMediaCookieAttributes(t *testing.T) {
	t.Parallel()
	r, _ := registerCookieMode(t)

	ck := mediaCookie(r)
	if ck == nil {
		t.Fatalf("cookie-mode register set no tessera_media cookie\nSet-Cookie: %v", r.Header.Values("Set-Cookie"))
	}
	if !ck.HttpOnly {
		t.Error("media cookie is not HttpOnly — script could read it, and it outlives the access token by 30 days")
	}
	if ck.Path != "/api/uploads" {
		t.Errorf("media cookie Path = %q, want /api/uploads — it must not ride along on every API call", ck.Path)
	}
	// Lax, not Strict: these images are also opened as plain links from mail and
	// chat, and Strict would blank those out. Nothing under /api/uploads changes
	// state — the upload route there is a POST that still requires a bearer.
	if ck.SameSite != http.SameSiteLaxMode {
		t.Errorf("media cookie SameSite = %v, want Lax", ck.SameSite)
	}
	if ck.MaxAge < 29*24*3600 {
		t.Errorf("media cookie MaxAge = %d, want the full 30-day session lifetime", ck.MaxAge)
	}
}

// Body mode is what Android, the desktop app and scripts use. They can't store
// a cookie usefully, so they must not be handed one.
func TestMediaCookieAbsentInBodyMode(t *testing.T) {
	t.Parallel()
	c := signup(t)

	r := authReq(t, http.MethodPost, "/auth/login", map[string]any{
		"email": c.Email, "password": "password-123",
	}, nil)
	if r.Status != http.StatusOK {
		t.Fatalf("login: status %d\n%s", r.Status, r.Body)
	}
	if ck := mediaCookie(r); ck != nil {
		t.Errorf("body-mode login set a tessera_media cookie (%q) — no existing client stores it", ck.Value)
	}
}

// Logging out has to take the image credential with it: it is the one cookie
// that would otherwise outlive the session by weeks.
func TestLogoutClearsMediaCookie(t *testing.T) {
	t.Parallel()
	_, refresh := registerCookieMode(t)

	r := authReq(t, http.MethodPost, "/auth/logout", nil, cookieMode, refresh)
	if r.Status != http.StatusNoContent {
		t.Fatalf("logout: status %d\n%s", r.Status, r.Body)
	}
	ck := mediaCookie(r)
	if ck == nil {
		t.Fatalf("logout did not clear tessera_media\nSet-Cookie: %v", r.Header.Values("Set-Cookie"))
	}
	if ck.Value != "" || ck.MaxAge >= 0 {
		t.Errorf("clearing cookie: value=%q MaxAge=%d, want an empty value and a negative MaxAge", ck.Value, ck.MaxAge)
	}
	if ck.Path != "/api/uploads" {
		t.Errorf("clearing cookie Path = %q — a mismatch leaves the live cookie in place", ck.Path)
	}
}

// The media token is signed with the same key as the access token but lives 30
// days. If it were accepted as a session credential, the hardening would hand
// out a month-long API session instead of taking one away.
func TestMediaTokenIsNotAnAccessToken(t *testing.T) {
	t.Parallel()
	r, _ := registerCookieMode(t)
	ck := mediaCookie(r)
	if ck == nil {
		t.Fatal("no media cookie to replay")
	}

	got := authReq(t, http.MethodGet, "/auth/me", nil, map[string]string{
		"Authorization": "Bearer " + ck.Value,
	})
	if got.Status != http.StatusUnauthorized {
		t.Fatalf("media token accepted on /auth/me: status %d\n%s", got.Status, got.Body)
	}
}

// mediaAuthServer boots a second API with MEDIA_REQUIRE_AUTH on. It shares the
// database and the upload directory with the harness, so a file uploaded through
// the shared server is the same file this one serves.
func mediaAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		JWTSecret:        "integration-test-secret-min32chars!!", // must match the harness: same tokens
		UploadDir:        testUploadDir,
		EncryptionKey:    "integration-test-encryption-key",
		PublicURL:        "http://tessera.test",
		CORSOrigin:       "*",
		RateLimitEnabled: false,
		MediaRequireAuth: true,
	}
	hub := realtime.NewHub()
	go hub.Run()
	r, _ := newRouter(cfg, testQueries, testPool, hub, mail.New(mail.Config{}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

// mediaGet fetches a media URL from the given server with the supplied
// credential, returning the status.
func mediaGet(t *testing.T, srv *httptest.Server, url string, headers map[string]string, cookies ...*http.Cookie) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode
}

// With enforcement on, every credential an actual client can present works, and
// nothing else does.
func TestMediaServeRequiresCredentialWhenEnforced(t *testing.T) {
	t.Parallel()
	srv := mediaAuthServer(t)

	// Upload through the shared server; both point at the same upload dir.
	c := signup(t)
	m := c.expect(t, uploadFile(t, c, "/uploads", "file", "секрет.png", []byte(pngBody)), http.StatusCreated)
	url, _ := m["url"].(string)
	if url == "" {
		t.Fatalf("upload returned no url: %v", m)
	}

	reg, _ := registerCookieMode(t)
	cookie := mediaCookie(reg)
	if cookie == nil {
		t.Fatal("no media cookie to present")
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(reg.Body, &body); err != nil {
		t.Fatalf("decode register: %v", err)
	}

	for _, tc := range []struct {
		name    string
		headers map[string]string
		cookies []*http.Cookie
		want    int
	}{
		{"anonymous", nil, nil, http.StatusUnauthorized},
		{"media cookie (web)", nil, []*http.Cookie{cookie}, http.StatusOK},
		{"bearer access token (android)", map[string]string{"Authorization": "Bearer " + body.AccessToken}, nil, http.StatusOK},
		{"forged cookie", nil, []*http.Cookie{{Name: "tessera_media", Value: "not-a-token"}}, http.StatusUnauthorized},
		{"garbage bearer", map[string]string{"Authorization": "Bearer nope"}, nil, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mediaGet(t, srv, url, tc.headers, tc.cookies...); got != tc.want {
				t.Fatalf("GET %s as %s: status %d, want %d", url, tc.name, got, tc.want)
			}
		})
	}

	// A missing file is still a 404 for an authorised caller — enforcement must
	// not turn every unknown name into an authentication answer.
	if got := mediaGet(t, srv, "/api/uploads/00000000-0000-0000-0000-000000000000.png", nil, cookie); got != http.StatusNotFound {
		t.Fatalf("authorised GET of a missing file: status %d, want 404", got)
	}
}

// The default install keeps serving what it serves today. This is the promise
// that makes the change safe to deploy: nothing breaks — desktop <img> tags
// included — until an operator deliberately turns MEDIA_REQUIRE_AUTH on.
func TestMediaServeStaysOpenByDefault(t *testing.T) {
	t.Parallel()
	c := signup(t)
	m := c.expect(t, uploadFile(t, c, "/uploads", "file", "открытая.png", []byte(pngBody)), http.StatusCreated)
	url, _ := m["url"].(string)

	if got := mediaGet(t, testServer, url, nil); got != http.StatusOK {
		t.Fatalf("anonymous GET %s on the default config: status %d, want 200", url, got)
	}
}
