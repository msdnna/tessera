package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// oauthBaseURL is what every OAuth redirect_uri and post-login handoff is built
// from. Behind a TLS-terminating edge (the dev Caddy on :8443, or any HTTPS
// reverse proxy) the last hop into the app is plain http, so the scheme has to
// come from X-Forwarded-Proto, not from the request. Getting it wrong sends the
// GitLab callback to http://host:8443 and the login dies (#2922).
func TestOAuthBaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name      string
		publicURL string
		host      string
		headers   map[string]string
		want      string
	}{
		{
			name: "https carried by a TLS-terminating proxy",
			host: "192.168.100.117:8443",
			headers: map[string]string{
				"X-Forwarded-Proto": "https",
				"X-Forwarded-Host":  "192.168.100.117:8443",
			},
			want: "https://192.168.100.117:8443",
		},
		{
			// A proxy chain appends; the client-facing scheme is the first token.
			name: "proxy chain lists the edge scheme first",
			host: "app.example:8443",
			headers: map[string]string{
				"X-Forwarded-Proto": "https, http",
				"X-Forwarded-Host":  "app.example:8443",
			},
			want: "https://app.example:8443",
		},
		{
			name:    "plain http dev frontend is left alone",
			host:    "localhost:8083",
			headers: map[string]string{"X-Forwarded-Proto": "http"},
			want:    "http://localhost:8083",
		},
		{
			name:      "configured PublicURL always wins",
			publicURL: "https://tessera.example/",
			host:      "backend:8080",
			headers:   map[string]string{"X-Forwarded-Proto": "http"},
			want:      "https://tessera.example",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &AuthHandler{publicURL: tc.publicURL}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			req := httptest.NewRequest("GET", "/api/auth/gitlab/authorize", nil)
			req.Host = tc.host
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			c.Request = req
			if got := h.oauthBaseURL(c); got != tc.want {
				t.Errorf("oauthBaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
