// Granting a membership has to reach the live socket too (#2932).
//
// A hub socket resolves its workspace scopes once, at the upgrade, and then
// lives for as long as the tab does. Every path that *takes* access away knows
// it — RemoveMember revokes, AddMember and AcceptInvitation drop the user's
// sockets so they come back re-scoped. The paths that *grant* it from the auth
// layer did not: an invitation applied by verifying the address (or by an OAuth
// sign-in) left the session holding a scope list from before the grant.
//
// The list of workspaces refetches over HTTP, so the newcomer sees the board —
// it just is not live. Nobody else's card moves, no notification arrives, and
// nothing says why; the workspace starts working after a reload that the user
// has no reason to perform.
package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestVerifiedInviteeGetsLiveWorkspace: after the pending invitation is applied
// by email verification, the session must end up receiving that workspace's
// events. Either outcome is fine — the socket is dropped and the client
// reconnects with fresh scopes, or it stays and already carries the workspace —
// and the failure is the third one: alive, silent, and stale.
func TestVerifiedInviteeGetsLiveWorkspace(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	st := mkStack(t, owner)

	email := fmt.Sprintf("rescope-%d@test.local", userSeq.Add(1))
	owner.expect(t, owner.post("/workspaces/"+st.WS+"/invitations",
		map[string]any{"email": email, "role": "member"}), http.StatusCreated)

	// Registration grants nothing (the address is not proven yet), so this socket
	// opens with the personal workspace as its only scope — exactly the tab the
	// invitee already has open when they go and click the link in the mail.
	late := registerWith(t, email, "")
	conn, status := dialWS(t, late.token, false)
	if conn == nil {
		t.Fatalf("invitee handshake failed (status %d)", status)
	}
	defer conn.Close()

	verifyEmailOf(t, late)
	if !listHasID(late.get("/workspaces").listBody(t), st.WS) {
		t.Fatal("верификация не применила приглашение — проверять нечего")
	}

	if awaitSocketClose(conn, 5*time.Second) {
		return // dropped on the grant: the reconnect picks the new scope up
	}
	// Still connected. Then it must already carry the workspace it was just let
	// into; a task created there is the cheapest event to look for.
	mkTask(t, owner, st.Board, st.col(t, 0), "Задача после верификации")
	if ev := readEvent(t, conn, 3*time.Second); ev == nil {
		t.Fatal("сокет пережил выдачу членства со старым списком пространств: " +
			"приглашение применено, доска нового пространства открывается, но события в неё не идут до перезагрузки")
	}
}
