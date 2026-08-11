package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// setCookie runs one of the cookie helpers against a fake request and returns the
// parsed tessera_refresh cookie.
func setCookie(t *testing.T, publicURL string, fn func(*AuthHandler, *gin.Context)) *http.Cookie {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	fn(&AuthHandler{publicURL: publicURL}, c)

	for _, ck := range w.Result().Cookies() {
		if ck.Name == refreshCookieName {
			return ck
		}
	}
	t.Fatalf("no %s cookie was set (Set-Cookie: %v)", refreshCookieName, w.Header().Values("Set-Cookie"))
	return nil
}

// Secure follows the public base URL, not the inbound request: behind nginx the
// request reaching Go is plain http even when the browser spoke https, so keying
// off c.Request.TLS would leave production cookies without Secure. The mistake in
// the other direction is just as quiet — Secure on a plain-http dev server makes
// the browser discard the cookie with no error anywhere.
func TestRefreshCookieSecureFollowsPublicURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		publicURL  string
		wantSecure bool
	}{
		{"https://tessera.example", true},
		{"https://tessera.example/", true},
		{"http://localhost:8090", false},
	}
	for _, tc := range cases {
		t.Run(tc.publicURL, func(t *testing.T) {
			ck := setCookie(t, tc.publicURL, func(h *AuthHandler, c *gin.Context) {
				h.setRefreshCookie(c, "the-token")
			})
			if ck.Secure != tc.wantSecure {
				t.Errorf("Secure = %v, want %v for PUBLIC_URL %q", ck.Secure, tc.wantSecure, tc.publicURL)
			}
			if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Path != refreshCookiePath {
				t.Errorf("attributes: HttpOnly=%v SameSite=%v Path=%q", ck.HttpOnly, ck.SameSite, ck.Path)
			}
			if ck.Value != "the-token" {
				t.Errorf("value = %q", ck.Value)
			}
		})
	}
}

// The clearing cookie must carry the same path and flags as the one it replaces,
// or the browser keeps the original alongside the expired one and the user stays
// logged in.
func TestClearRefreshCookieMatchesAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ck := setCookie(t, "https://tessera.example", func(h *AuthHandler, c *gin.Context) {
		h.clearRefreshCookie(c)
	})
	if ck.Value != "" || ck.MaxAge >= 0 {
		t.Errorf("clearing cookie: value=%q MaxAge=%d, want empty value and a negative MaxAge", ck.Value, ck.MaxAge)
	}
	if ck.Path != refreshCookiePath || !ck.HttpOnly || !ck.Secure {
		t.Errorf("clearing cookie attributes differ from the original: Path=%q HttpOnly=%v Secure=%v",
			ck.Path, ck.HttpOnly, ck.Secure)
	}
}

// The mode switch is explicit and case-insensitive; anything else means the
// legacy body delivery, which is what every existing client relies on.
func TestWantsCookieAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := map[string]bool{"cookie": true, "COOKIE": true, "": false, "body": false, "cookies": false}
	for header, want := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		if header != "" {
			c.Request.Header.Set(authModeHeader, header)
		}
		if got := wantsCookieAuth(c); got != want {
			t.Errorf("wantsCookieAuth(%q) = %v, want %v", header, got, want)
		}
	}
}
