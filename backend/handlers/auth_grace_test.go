package handlers

import (
	"testing"
	"time"

	"tessera/internal/db"
)

// #2750. Refresh rotation revokes the presented token and only then writes the
// new pair — a connection dropped in between leaves the client holding a token
// the server already killed, and the user is signed out by a lost response.
// refreshTokenUsable is the grace window that answers that retry.
func TestRefreshTokenUsable(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time {
		tm := now.Add(-d)
		return &tm
	}

	cases := []struct {
		name       string
		rt         db.RefreshToken
		wantUsable bool
		wantReplay bool
	}{
		{
			name:       "live token",
			rt:         db.RefreshToken{ExpiresAt: now.Add(24 * time.Hour)},
			wantUsable: true,
		},
		{
			name:       "revoked just now is a replay of a lost rotation",
			rt:         db.RefreshToken{ExpiresAt: now.Add(24 * time.Hour), RevokedAt: ago(5 * time.Second)},
			wantUsable: true,
			wantReplay: true,
		},
		{
			name:       "revoked at the edge of the window still counts",
			rt:         db.RefreshToken{ExpiresAt: now.Add(24 * time.Hour), RevokedAt: ago(refreshGrace)},
			wantUsable: true,
			wantReplay: true,
		},
		{
			name: "revoked past the window is rejected",
			rt:   db.RefreshToken{ExpiresAt: now.Add(24 * time.Hour), RevokedAt: ago(refreshGrace + time.Second)},
		},
		{
			name: "revoked long ago is rejected",
			rt:   db.RefreshToken{ExpiresAt: now.Add(24 * time.Hour), RevokedAt: ago(5 * time.Minute)},
		},
		{
			name: "expired token is rejected even though it was never revoked",
			rt:   db.RefreshToken{ExpiresAt: now.Add(-time.Second)},
		},
		{
			// Logout/password-reset pull expires_at into the past precisely so the
			// grace window cannot resurrect a deliberately ended session.
			name: "hard-revoked (expiry pulled back) beats the grace window",
			rt:   db.RefreshToken{ExpiresAt: now.Add(-time.Second), RevokedAt: ago(time.Second)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			usable, replay := refreshTokenUsable(tc.rt, now)
			if usable != tc.wantUsable || replay != tc.wantReplay {
				t.Fatalf("refreshTokenUsable() = (usable=%v, replay=%v), want (usable=%v, replay=%v)",
					usable, replay, tc.wantUsable, tc.wantReplay)
			}
		})
	}
}
