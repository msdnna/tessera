package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// The one-time code replaces the tokens in the desktop deep link because that link
// becomes argv (#2932). The replacement is only worth anything if the code itself is
// short-lived, single-use and unguessable — these tests are the ratchet on all three.

func TestOAuthHandoffCodeIsSingleUse(t *testing.T) {
	s := newOAuthHandoffStore()
	user := uuid.New()
	now := time.Now()

	code, err := s.issue(user, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, ok := s.redeem(code, now)
	if !ok || got != user {
		t.Fatalf("first redemption failed: got %v, ok=%v (want %v)", got, ok, user)
	}
	// A deep link stays readable after the app has used it — in the shell history, in
	// the portal log, in whatever captured the argv. Replaying it must buy nothing.
	if _, ok := s.redeem(code, now); ok {
		t.Fatal("the code was accepted a second time — a captured deep link is a reusable login")
	}
}

func TestOAuthHandoffCodeExpires(t *testing.T) {
	s := newOAuthHandoffStore()
	now := time.Now()
	code, err := s.issue(uuid.New(), now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	if _, ok := s.redeem(code, now.Add(oauthHandoffTTL-time.Second)); !ok {
		t.Fatal("a code inside its TTL was rejected — real logins would fail")
	}

	code2, _ := s.issue(uuid.New(), now)
	if _, ok := s.redeem(code2, now.Add(oauthHandoffTTL+time.Second)); ok {
		t.Fatal("an expired code was accepted")
	}
	// Expired or not, the attempt must consume the entry: otherwise a clock skew or a
	// later sweep could still leave it redeemable.
	s.mu.Lock()
	_, still := s.pending[code2]
	s.mu.Unlock()
	if still {
		t.Fatal("an expired code stayed in the store after a redemption attempt")
	}
}

func TestOAuthHandoffRejectsUnknownCodes(t *testing.T) {
	s := newOAuthHandoffStore()
	now := time.Now()
	if _, err := s.issue(uuid.New(), now); err != nil {
		t.Fatalf("issue: %v", err)
	}
	for _, code := range []string{"", "deadbeef", strings.Repeat("0", 64)} {
		if _, ok := s.redeem(code, now); ok {
			t.Fatalf("redeemed a code nobody issued: %q", code)
		}
	}
}

// Codes are guessed offline against a public endpoint, so they have to be both unique
// and long. 64 hex chars = 256 bits.
func TestOAuthHandoffCodesAreUniqueAndLong(t *testing.T) {
	s := newOAuthHandoffStore()
	now := time.Now()
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := s.issue(uuid.New(), now)
		if err != nil {
			t.Fatalf("issue: %v", err)
		}
		if len(code) != 64 {
			t.Fatalf("code is %d chars, want 64: %q", len(code), code)
		}
		if seen[code] {
			t.Fatalf("issued a duplicate code: %q", code)
		}
		seen[code] = true
	}
}

// Nothing sweeps the map on a timer, so an abandoned login has to be cleaned up by the
// next one — otherwise a login screen left open all day grows the map forever.
func TestOAuthHandoffSweepsAbandonedCodes(t *testing.T) {
	s := newOAuthHandoffStore()
	now := time.Now()
	for i := 0; i < 5; i++ {
		if _, err := s.issue(uuid.New(), now); err != nil {
			t.Fatalf("issue: %v", err)
		}
	}
	if _, err := s.issue(uuid.New(), now.Add(oauthHandoffTTL+time.Second)); err != nil {
		t.Fatalf("issue: %v", err)
	}
	s.mu.Lock()
	n := len(s.pending)
	s.mu.Unlock()
	if n != 1 {
		t.Fatalf("store holds %d entries after the abandoned ones expired, want 1", n)
	}
}

// The endpoint is public, so the rejection path must not need a database — and must not
// tell an attacker which of unknown/expired/spent they hit.
func TestOAuthHandoffEndpointRejectsBadCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AuthHandler{handoff: newOAuthHandoffStore(), secret: testSecret}

	cases := []struct {
		name string
		body string
		want int
	}{
		{"unknown code", `{"code":"deadbeef"}`, http.StatusUnauthorized},
		{"empty code", `{"code":""}`, http.StatusUnauthorized},
		{"no code field", `{}`, http.StatusUnauthorized},
		{"not json", `nope`, http.StatusBadRequest},
	}
	var bodies []string
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/auth/oauth/handoff", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			h.OAuthHandoff(c)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
			if w.Code == http.StatusUnauthorized {
				bodies = append(bodies, w.Body.String())
			}
		})
	}
	for _, b := range bodies[1:] {
		if b != bodies[0] {
			t.Fatalf("rejection messages differ (%s vs %s) — that tells a guesser which codes exist", bodies[0], b)
		}
	}
}
