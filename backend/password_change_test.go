// Changing your own password must end every other session (#2932).
//
// The reset flow already did this — ResetPassword revokes the whole refresh
// chain — but the in-session change did not, and that is the one a user reaches
// for when they suspect someone else is in the account. With refresh tokens
// rotating on a 30-day lifetime, the stolen session simply renewed itself past
// the new password forever. These pin that the two paths end the same way, and
// that the user who performed the change is not thrown out of their own device
// in the process.
package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// sessionOf logs the user in again and returns the second session's tokens —
// this stands in for the credential an attacker walked off with.
func sessionOf(t *testing.T, email, password string) (access, refresh string) {
	t.Helper()
	r := login(t, email, password)
	if r.Status != http.StatusOK {
		t.Fatalf("login: status %d\n%s", r.Status, r.Body)
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("login decode: %v\n%s", err, r.Body)
	}
	if out.RefreshToken == "" {
		t.Fatalf("login returned no refresh token: %s", r.Body)
	}
	return out.AccessToken, out.RefreshToken
}

// TestPasswordChangeEndsOtherSessions is the whole point of the endpoint: after
// the change, the other session must be unable to renew itself.
func TestPasswordChangeEndsOtherSessions(t *testing.T) {
	t.Parallel()
	victim := signup(t)
	stolenAccess, stolenRefresh := sessionOf(t, victim.Email, "password-123")

	// Sanity: the second session is live before the change, so a dead token
	// afterwards is the change's doing and not a broken fixture.
	if r := doReq(t, stolenAccess, http.MethodGet, "/auth/me", nil); r.Status != http.StatusOK {
		t.Fatalf("second session before change: status %d\n%s", r.Status, r.Body)
	}

	r := victim.put("/users/me/password", map[string]any{
		"current_password": "password-123", "new_password": "password-456",
	})
	if r.Status != http.StatusOK && r.Status != http.StatusNoContent {
		t.Fatalf("change password: status %d\n%s", r.Status, r.Body)
	}

	// The rotating credential is the one that matters: the access token expires
	// in 15 minutes on its own, but a refresh token that still works renews the
	// intruder's access for another month, and then another.
	if rr := doReq(t, "", http.MethodPost, "/auth/refresh",
		map[string]any{"refresh_token": stolenRefresh}); rr.Status == http.StatusOK {
		t.Fatalf("session from the old password refreshed itself after the change: %s", rr.Body)
	}

	// The old password is gone too — otherwise the intruder just logs back in.
	if rr := login(t, victim.Email, "password-123"); rr.Status == http.StatusOK {
		t.Fatalf("old password still logs in after the change: %s", rr.Body)
	}
	if rr := login(t, victim.Email, "password-456"); rr.Status != http.StatusOK {
		t.Fatalf("new password does not log in: status %d\n%s", rr.Status, rr.Body)
	}
}

// TestPasswordChangeKeepsCallerSignedIn covers the other half. Revoking every
// token is easy; doing it without logging the user out of the device they are
// holding is the part that needs the response to carry a fresh pair. A client
// left with a dead refresh token would drop to the login screen minutes later,
// long after the success toast, which reads as a bug rather than as security.
func TestPasswordChangeKeepsCallerSignedIn(t *testing.T) {
	t.Parallel()
	c := signup(t)

	r := c.put("/users/me/password", map[string]any{
		"current_password": "password-123", "new_password": "password-456",
	})
	if r.Status != http.StatusOK {
		t.Fatalf("change password: status %d, want 200 with a fresh session\n%s", r.Status, r.Body)
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("decode: %v\n%s", err, r.Body)
	}
	if out.AccessToken == "" || out.RefreshToken == "" {
		t.Fatalf("no replacement session in the response: %s", r.Body)
	}
	if rr := doReq(t, out.AccessToken, http.MethodGet, "/auth/me", nil); rr.Status != http.StatusOK {
		t.Fatalf("replacement access token: status %d\n%s", rr.Status, rr.Body)
	}
	if rr := doReq(t, "", http.MethodPost, "/auth/refresh",
		map[string]any{"refresh_token": out.RefreshToken}); rr.Status != http.StatusOK {
		t.Fatalf("replacement refresh token: status %d\n%s", rr.Status, rr.Body)
	}
	// The pair the caller arrived with is revoked along with everyone else's —
	// the replacement is a new chain, not the old one waved through.
	if rr := doReq(t, "", http.MethodPost, "/auth/refresh",
		map[string]any{"refresh_token": c.Refresh}); rr.Status == http.StatusOK {
		t.Fatalf("the caller's pre-change refresh token survived: %s", rr.Body)
	}
}

// TestPasswordChangeKeepsPATs draws the line on the other side. Personal access
// tokens are separate credentials with their own revoke button, living in CI
// configs and MCP clients; sweeping them away on a routine password rotation
// would break unattended jobs with no way for the user to connect cause and
// effect. Reset does not touch them either.
func TestPasswordChangeKeepsPATs(t *testing.T) {
	t.Parallel()
	c := signup(t)
	pat := mkPAT(t, c, "ci-runner")

	if r := c.put("/users/me/password", map[string]any{
		"current_password": "password-123", "new_password": "password-456",
	}); r.Status != http.StatusOK && r.Status != http.StatusNoContent {
		t.Fatalf("change password: status %d\n%s", r.Status, r.Body)
	}

	if r := pat.get("/auth/me"); r.Status != http.StatusOK {
		t.Fatalf("PAT after a password change: status %d, want 200\n%s", r.Status, r.Body)
	}
}

// TestPasswordChangeRejectsWrongCurrent pins the guard that makes all of the
// above safe to run from a live session: without it a stolen access token could
// itself rotate the password and lock the owner out.
func TestPasswordChangeRejectsWrongCurrent(t *testing.T) {
	t.Parallel()
	c := signup(t)

	if r := c.put("/users/me/password", map[string]any{
		"current_password": "not-the-password", "new_password": "password-456",
	}); r.Status != http.StatusForbidden {
		t.Fatalf("wrong current password: status %d, want 403\n%s", r.Status, r.Body)
	}
	// A refused attempt changes nothing — the session it was made from stays up.
	if r := doReq(t, "", http.MethodPost, "/auth/refresh",
		map[string]any{"refresh_token": c.Refresh}); r.Status != http.StatusOK {
		t.Fatalf("refresh after a refused change: status %d, want 200\n%s", r.Status, r.Body)
	}
}
