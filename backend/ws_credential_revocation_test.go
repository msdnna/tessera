// Credential revocation has to reach the WebSocket, not just the refresh chain
// (#2932). A socket authenticates exactly once, at the upgrade, and then lives
// on ping/pong indefinitely — so "change my password", the one control the
// interface offers someone who has noticed an intruder, left that intruder's
// open tab streaming every event of every workspace the account belongs to.
package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// awaitSocketClosed fails unless the server tears the connection down within
// the deadline. Exactly one read: gorilla panics on a second read after a
// failed one, so a timeout here is the verdict rather than a reason to retry —
// and a timeout *is* the failure, because a live socket is one that goes on
// delivering the account's events.
func awaitSocketClosed(t *testing.T, conn *websocket.Conn, what string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("%s: socket is still live and delivering frames", what)
	}
	if strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("%s: socket was never closed — it keeps streaming this account's events", what)
	}
}

// TestWSDropsSocketsOnPasswordChange: the in-session password change must cut
// the sockets the old password was behind, the same way it already cuts the
// refresh chain.
func TestWSDropsSocketsOnPasswordChange(t *testing.T) {
	victim := signup(t)
	mkStack(t, victim)

	// The intruder's tab: a socket opened on the stolen session, before the
	// victim reacts.
	conn, status := dialWS(t, victim.token, false)
	if conn == nil {
		t.Fatalf("handshake failed (status %d)", status)
	}
	defer conn.Close()

	r := victim.put("/users/me/password", map[string]any{
		"current_password": "password-123", "new_password": "new-password-456",
	})
	if r.Status != http.StatusOK && r.Status != http.StatusNoContent {
		t.Fatalf("change password: status %d\n%s", r.Status, r.Body)
	}

	awaitSocketClosed(t, conn, "password change")
}

// TestWSDropsSocketsOnPasswordReset covers the same act through the other door
// — the reset link, which is where a user who cannot get in at all ends up.
func TestWSDropsSocketsOnPasswordReset(t *testing.T) {
	victim := signup(t)
	mkStack(t, victim)

	conn, status := dialWS(t, victim.token, false)
	if conn == nil {
		t.Fatalf("handshake failed (status %d)", status)
	}
	defer conn.Close()

	raw := insertUserToken(t, victim.UserID, "reset")
	r := doReq(t, "", http.MethodPost, "/auth/reset-password", map[string]any{
		"token": raw, "new_password": "reset-password-789",
	})
	if r.Status != http.StatusNoContent {
		t.Fatalf("reset: status %d\n%s", r.Status, r.Body)
	}

	awaitSocketClosed(t, conn, "password reset")
}

// TestWSSurvivesAnotherUsersPasswordChange is the negative control: revocation
// must be aimed at one account, not at everyone connected.
func TestWSSurvivesAnotherUsersPasswordChange(t *testing.T) {
	bystander := signup(t)
	other := signup(t)
	st := mkStack(t, bystander)

	conn, status := dialWS(t, bystander.token, false)
	if conn == nil {
		t.Fatalf("handshake failed (status %d)", status)
	}
	defer conn.Close()

	r := other.put("/users/me/password", map[string]any{
		"current_password": "password-123", "new_password": "new-password-456",
	})
	if r.Status != http.StatusOK && r.Status != http.StatusNoContent {
		t.Fatalf("change password: status %d\n%s", r.Status, r.Body)
	}

	mkTask(t, bystander, st.Board, st.Columns[0]["id"].(string), "После чужой смены пароля")
	if ev := readEvent(t, conn, 5*time.Second); ev == nil {
		t.Fatal("a bystander's socket died when someone else changed their password")
	}
}
