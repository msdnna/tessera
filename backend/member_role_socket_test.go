// A role change has to reach the session it changes (#2932).
//
// Membership — the workspace set and the role inside each — is snapshotted at
// the WebSocket upgrade and never re-read on a live connection. Hub.DropUser is
// the whole mechanism by which a membership change reaches a client that is
// already connected: it closes the socket, the client reconnects, and on
// reconnect it re-reads what it is. Every door that changes a membership fires
// it — AddMember (the same role change by upsert), AcceptInvitation,
// RemoveMember via revokeUserSockets — except UpdateMemberRole, which until now
// returned 200 and told nobody. A member promoted to admin went on seeing the
// members dialog without its invite form, and a demoted admin went on seeing one
// whose every control answers 403, until they reloaded the page.
package main

import (
	"net/http"
	"testing"
	"time"
)

// TestWSDropsRoleChangedMember: PATCH .../members/:userId must cut the target's
// live sockets, and must not do anything more than that — they are still a
// member, and the role they now hold is the new one.
func TestWSDropsRoleChangedMember(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	st := mkStack(t, owner)
	member := addMember(t, owner, st.WS)

	conn, status := dialWS(t, member.token, false)
	if conn == nil {
		t.Fatalf("member handshake failed (status %d)", status)
	}
	defer conn.Close()

	owner.expect(t, owner.patch("/workspaces/"+st.WS+"/members/"+member.UserID,
		map[string]any{"role": "admin"}), http.StatusOK)

	if !awaitSocketClose(conn, 5*time.Second) {
		t.Fatal("socket of a member whose role changed was never closed")
	}

	// Cutting the socket is a nudge, not a revocation: nothing was taken away
	// here, so the reconnect the drop provokes must succeed — and it is the
	// workspace list it re-reads, so that is where the new role has to show up.
	back, status := dialWS(t, member.token, false)
	if back == nil {
		t.Fatalf("promoted member could not reconnect (status %d)", status)
	}
	defer back.Close()

	for _, w := range member.get("/workspaces").listBody(t) {
		if w["id"] != st.WS {
			continue
		}
		if w["my_role"] != "admin" {
			t.Fatalf("my_role after the promotion = %v, want admin", w["my_role"])
		}
		return
	}
	t.Fatal("the promoted member no longer sees the workspace at all")
}
