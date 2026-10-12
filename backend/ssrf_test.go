// Integration tests for #2624/#2625: the GitLab base URL is user-controlled
// input, so ConnectGitlab rejects a non-http(s) / userinfo / hostless
// destination up front with a clear 400. The transport's guarded dialer is the
// backstop for a host that resolves to a private address (covered by the
// netguard package's own dial test).
package main

import (
	"net/http"
	"testing"
)

func TestConnectGitlabRejectsBadBaseURL(t *testing.T) {
	t.Parallel()
	c := signup(t)

	for _, tc := range []struct{ name, url string }{
		{"file scheme", "file:///etc/passwd"},
		{"javascript scheme", "javascript:alert(1)"},
		{"gopher scheme", "gopher://x/"},
		{"userinfo", "https://user:pass@gitlab.example.com"},
		{"hostless", "https:///path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := c.post("/gitlab/connection", map[string]any{"base_url": tc.url, "token": "x"})
			if r.Status != http.StatusBadRequest {
				t.Fatalf("base_url %q: status %d, want 400\n%s", tc.url, r.Status, r.Body)
			}
		})
	}
}

// The instance-wide OAuth provider's base URL gets the same up-front check: the
// server calls it with the service token and every OAuth code exchange. A 400
// writes nothing, so the shared provider row is untouched.
func TestAdminOAuthRejectsBadBaseURL(t *testing.T) {
	c := signup(t)
	makeAdmin(t, c)
	for _, url := range []string{"file:///etc/passwd", "https://user:pass@gitlab.example.com", "https:///path"} {
		r := c.put("/admin/oauth/gitlab", map[string]any{"gl_base_url": url, "client_id": "x"})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("gl_base_url %q: status %d, want 400\n%s", url, r.Status, r.Body)
		}
	}
}
