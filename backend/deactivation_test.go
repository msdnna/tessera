// Deactivating an account must end the access it already has (#2932).
//
// `active` was read in exactly two places — password login and the OAuth
// callback — and both are doors the deactivated user no longer needs: they are
// already inside. The refresh endpoint minted a fresh pair for them forever, the
// PAT they left behind kept full API access, and their live socket kept
// streaming. Offboarding is the whole point of the switch, so these pin that it
// reaches every live credential, not just the login form.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// setActiveInDB flips the flag behind the API's back. The handler-level test
// covers the switch as an admin uses it; these tests need the other half —
// whether the *credential* paths re-check the flag on their own, with no
// revocation having run.
func setActiveInDB(t *testing.T, userID string, active bool) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET active = $2 WHERE id = $1`, userID, active); err != nil {
		t.Fatalf("set active=%v: %v", active, err)
	}
}

// makeGlobalAdmin promotes a signed-up user to instance admin. Registration only
// hands that out to the very first account, which on a shared test database was
// claimed long ago.
func makeGlobalAdmin(t *testing.T, userID string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = true WHERE id = $1`, userID); err != nil {
		t.Fatalf("promote to admin: %v", err)
	}
}

// mkPAT mints a personal access token for the client and returns a client that
// authenticates with it.
func mkPAT(t *testing.T, c *client, name string) *client {
	t.Helper()
	m := c.expect(t, c.post("/auth/tokens", map[string]any{"name": name}), http.StatusCreated)
	token, _ := m["token"].(string)
	if token == "" {
		t.Fatalf("no PAT in response: %v", m)
	}
	return &client{t: t, token: token, UserID: c.UserID, Email: c.Email}
}

// TestDeactivationEndsLiveAccess walks the switch the way an admin does: the
// account is deactivated through the API, and everything the departing user is
// still holding has to stop working — the rotating session, the headless token,
// the open socket.
func TestDeactivationEndsLiveAccess(t *testing.T) {
	t.Parallel()
	admin := signup(t)
	makeGlobalAdmin(t, admin.UserID)

	leaver := signup(t)
	pat := mkPAT(t, leaver, "offboarding")

	conn, status := dialWS(t, leaver.token, false)
	if conn == nil {
		t.Fatalf("leaver handshake = %d", status)
	}
	defer conn.Close()

	if r := admin.patch("/admin/users/"+leaver.UserID+"/active", map[string]any{"active": false}); r.Status != http.StatusNoContent {
		t.Fatalf("deactivate: status %d\n%s", r.Status, r.Body)
	}

	// 1. The rotating session. Without this the account never actually leaves:
	// a 30-day refresh token renews itself indefinitely and no one ever notices.
	r := doReq(t, "", http.MethodPost, "/auth/refresh", map[string]any{"refresh_token": leaver.Refresh})
	if r.Status == http.StatusOK {
		t.Fatalf("deactivated user refreshed their session: %s", r.Body)
	}
	if r.Status != http.StatusUnauthorized && r.Status != http.StatusForbidden {
		t.Fatalf("refresh after deactivation: status %d, want 401/403\n%s", r.Status, r.Body)
	}

	// 2. The headless credential. PATs are the long-lived ones — months, and
	// they sit in CI configs and MCP clients, far from anyone's laptop.
	if r := pat.get("/auth/me"); r.Status != http.StatusUnauthorized {
		t.Fatalf("PAT of a deactivated user: status %d, want 401\n%s", r.Status, r.Body)
	}

	// 3. The socket that is already open. It carries the board stream for every
	// workspace the user was in and nothing re-authorises it after the upgrade.
	if !awaitSocketClose(conn, 5*time.Second) {
		t.Fatal("socket of a deactivated user was never closed")
	}

	// The login form, which is all the flag used to guard, still says no.
	if r := login(t, leaver.Email, "password-123"); r.Status != http.StatusForbidden {
		t.Fatalf("login after deactivation: status %d, want 403\n%s", r.Status, r.Body)
	}

	// Reactivation gives the account back whole: deactivation is a pause, not a
	// shredder, so the PAT the user still holds must work again.
	if r := admin.patch("/admin/users/"+leaver.UserID+"/active", map[string]any{"active": true}); r.Status != http.StatusNoContent {
		t.Fatalf("reactivate: status %d\n%s", r.Status, r.Body)
	}
	if r := pat.get("/auth/me"); r.Status != http.StatusOK {
		t.Fatalf("PAT after reactivation: status %d, want 200\n%s", r.Status, r.Body)
	}
	// The session, however, does not come back — those tokens were revoked.
	if r := doReq(t, "", http.MethodPost, "/auth/refresh",
		map[string]any{"refresh_token": leaver.Refresh}); r.Status == http.StatusOK {
		t.Fatalf("revoked refresh token revived by reactivation: %s", r.Body)
	}
}

// TestRefreshRejectsDeactivatedUser pins the refresh endpoint on its own, with
// the flag flipped straight in the database so no revocation has run. The
// revocation on the admin path is a convenience — it ends the session at once
// instead of within the access token's lifetime — but a refresh token that
// escaped it (minted during the request, restored from a backup) must still be
// refused on the merits of the account.
func TestRefreshRejectsDeactivatedUser(t *testing.T) {
	t.Parallel()
	c := signup(t)
	setActiveInDB(t, c.UserID, false)

	r := doReq(t, "", http.MethodPost, "/auth/refresh", map[string]any{"refresh_token": c.Refresh})
	if r.Status != http.StatusForbidden {
		t.Fatalf("refresh for deactivated account: status %d, want 403\n%s", r.Status, r.Body)
	}

	setActiveInDB(t, c.UserID, true)
	if r := doReq(t, "", http.MethodPost, "/auth/refresh",
		map[string]any{"refresh_token": c.Refresh}); r.Status != http.StatusOK {
		t.Fatalf("refresh for a live account: status %d, want 200\n%s", r.Status, r.Body)
	}
}

// TestPATRejectedForDeactivatedOwner pins the bearer path the same way: a token
// is only as authorised as the account behind it, and that account can be turned
// off long after the token was minted.
func TestPATRejectedForDeactivatedOwner(t *testing.T) {
	t.Parallel()
	c := signup(t)
	pat := mkPAT(t, c, "ci-runner")
	if r := pat.get("/auth/me"); r.Status != http.StatusOK {
		t.Fatalf("fresh PAT: status %d, want 200\n%s", r.Status, r.Body)
	}

	setActiveInDB(t, c.UserID, false)
	if r := pat.get("/auth/me"); r.Status != http.StatusUnauthorized {
		t.Fatalf("PAT of a deactivated owner: status %d, want 401\n%s", r.Status, r.Body)
	}
	// Not just /auth/me: the credential is refused at the middleware, so every
	// protected route goes with it.
	if r := pat.get("/workspaces"); r.Status != http.StatusUnauthorized {
		t.Fatalf("PAT on /workspaces: status %d, want 401\n%s", r.Status, r.Body)
	}

	setActiveInDB(t, c.UserID, true)
	if r := pat.get("/auth/me"); r.Status != http.StatusOK {
		t.Fatalf("PAT after reactivation: status %d, want 200\n%s", r.Status, r.Body)
	}
}
