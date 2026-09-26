package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"tessera/config"
)

// The rate limiter keys on c.ClientIP(), and gin only believes a forwarded-for
// header when the *direct peer* is in the trusted-proxy list. In the shipped
// docker deployment that peer is the nginx container, reached over the compose
// network — a private address, never loopback. With the list defaulting to
// loopback alone, every request from the internet arrived with the same
// ClientIP: the nginx container's. One global bucket for /auth/login,
// /auth/register and /auth/refresh, so ten logins from anyone in six seconds
// answered 429 to the whole instance, and the access log recorded one address
// for every caller (#2932).
//
// The other half of the same bug lives in frontend/nginx.conf, which set
// X-Real-IP but no X-Forwarded-For — see frontend/tests/nginxForwardedFor.spec.js.
// Both halves are needed: gin reads X-Forwarded-For first, so trusting the proxy
// without having it write that header is what would let a caller pick its own
// bucket by sending one.

// edgeRouter mirrors how router.go mounts the limiter, including the shipped
// trusted-proxy list — the value under test here.
func edgeRouter(t *testing.T, rules map[string]RateRule) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := r.SetTrustedProxies(config.DefaultTrustedProxies); err != nil {
		t.Fatalf("SetTrustedProxies(%v): %v", config.DefaultTrustedProxies, err)
	}
	r.Use(RateLimit(rules))
	r.POST("/api/auth/login", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

// postFrom sends a login through the router as `peer` would deliver it, carrying
// the forwarded chain nginx writes ($proxy_add_x_forwarded_for appends the peer
// it saw to whatever the client sent, so the real client is leftmost).
func postFrom(r *gin.Engine, peer, forwardedFor, email string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
		bytes.NewBufferString(`{"email":"`+email+`"}`))
	req.Header.Set("Content-Type", "application/json")
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	req.RemoteAddr = peer + ":40000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Two unrelated people logging in through the same reverse proxy must not share
// a budget.
func TestRateLimitKeepsClientsApartBehindTheContainerProxy(t *testing.T) {
	const nginx = "172.18.0.4" // the frontend container on the compose network
	rules := map[string]RateRule{"/api/auth/login": {Every: 6 * time.Second, Burst: 2}}
	r := edgeRouter(t, rules)

	// The first caller spends the whole burst.
	for i := range 2 {
		if got := postFrom(r, nginx, "203.0.113.7", "first@example.test").Code; got != http.StatusOK {
			t.Fatalf("first caller, request %d: got %d, want 200", i+1, got)
		}
	}
	if got := postFrom(r, nginx, "203.0.113.7", "first@example.test").Code; got != http.StatusTooManyRequests {
		t.Fatalf("first caller past its burst: got %d, want 429", got)
	}
	// A different person, same proxy. Their own budget is untouched.
	if got := postFrom(r, nginx, "198.51.100.9", "second@example.test").Code; got != http.StatusOK {
		t.Fatalf("a second client behind the same proxy was throttled by the first one's "+
			"budget (got %d, want 200): every caller shares one bucket", got)
	}
}

// The mirror image: a caller that reaches the backend directly must not be able
// to mint itself a fresh bucket by forging the header. This is what stops the
// fix above from being "trust every proxy".
func TestRateLimitIgnoresForwardedForFromAnUntrustedPeer(t *testing.T) {
	rules := map[string]RateRule{"/api/auth/login": {Every: 6 * time.Second, Burst: 2}}
	r := edgeRouter(t, rules)

	for i := range 2 {
		if got := postFrom(r, "203.0.113.7", "", "spray@example.test").Code; got != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i+1, got)
		}
	}
	for i, forged := range []string{"10.1.2.3", "198.51.100.1", "8.8.8.8"} {
		if got := postFrom(r, "203.0.113.7", forged, "spray@example.test").Code; got != http.StatusTooManyRequests {
			t.Fatalf("forged X-Forwarded-For %q (attempt %d) bought a fresh budget: got %d, want 429",
				forged, i+1, got)
		}
	}
}
