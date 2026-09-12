// Revoking a membership must cut the room sockets too (#2932).
//
// TestWSDropsRemovedMember already pins the board socket: RemoveMember calls
// Hub.DropUser and the connection dies. The per-document and per-conference
// sockets are a separate fan-out with a separate registry, and they authorise
// exactly once — at the upgrade. Nothing re-reads the membership afterwards and
// nothing closes them on removal, so until this file they outlived the access
// that justified them for as long as the tab stayed open.
package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// awaitSocketClose reports whether the socket died within the budget. Any error
// counts: a close frame, a reset, a closed transport — the point is that the
// server stopped talking to this connection, not how it said so.
// One deadline for the whole wait, not one per read: gorilla marks a connection
// failed on the first read error — including its own deadline — and panics on
// the next read, so a poll loop with short deadlines cannot ask twice.
func awaitSocketClose(conn *websocket.Conn, budget time.Duration) bool {
	_ = conn.SetReadDeadline(time.Now().Add(budget))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				return false // still talking when the budget ran out
			}
			return true
		}
	}
}

// TestRemovedMemberLosesDocumentSocket: the ex-member's document socket must go
// with their access. While it lives they keep presence in someone else's
// document, keep being told when it is saved, and — worse — keep holding block
// locks, which is a veto on everyone else's editing that no one in the workspace
// can lift.
func TestRemovedMemberLosesDocumentSocket(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	wsID, docID := mkDoc(t, owner, "Документ, из которого исключают")
	mate := addMember(t, owner, wsID)

	stayer, status := dialDocWS(t, owner.token, docID)
	if stayer == nil {
		t.Fatalf("owner handshake = %d", status)
	}
	defer stayer.Close()
	leaver, status := dialDocWS(t, mate.token, docID)
	if leaver == nil {
		t.Fatalf("member handshake = %d", status)
	}
	defer leaver.Close()
	if awaitFrame(t, leaver, "welcome", 5*time.Second) == nil {
		t.Fatal("no welcome for the member")
	}

	// A held lock is what makes this more than a presence leak: the block stays
	// unavailable to the workspace until the ex-member's tab happens to close.
	if err := leaver.WriteJSON(map[string]any{"type": "lock", "block_id": "b1"}); err != nil {
		t.Fatalf("lock request: %v", err)
	}
	held := awaitState(t, stayer, 5*time.Second, func(m map[string]any) bool {
		return lockNames(m)["b1"] != ""
	})
	if held == nil {
		t.Fatal("the member's lock never reached the room snapshot")
	}

	if r := owner.del("/workspaces/" + wsID + "/members/" + mate.UserID); r.Status != http.StatusNoContent {
		t.Fatalf("remove member: status %d\n%s", r.Status, r.Body)
	}

	if !awaitSocketClose(leaver, 5*time.Second) {
		t.Fatal("the document socket of a removed member was never closed")
	}
	// Closing the socket is only half the repair: the room has to release what
	// that connection held, or the block stays locked by a ghost.
	freed := awaitState(t, stayer, 5*time.Second, func(m map[string]any) bool {
		viewers, _ := m["viewers"].([]any)
		return len(viewers) == 1 && lockNames(m)["b1"] == ""
	})
	if freed == nil {
		t.Fatal("the removed member kept their place and their lock in the room")
	}
	// Control that the removal itself took effect: a fresh dial is refused.
	if conn, status := dialDocWS(t, mate.token, docID); status != http.StatusForbidden {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("a removed member re-dialled the document: status %d, want 403", status)
	}
}

// TestRemovedMemberLosesConferenceSocket: the same for a live call, where the
// stale socket is worse than presence. A member who joined as the call's host —
// or as a workspace admin — was handed moderation rights at the upgrade, and
// keeps kicking and force-muting the people still in the meeting after being
// removed from the workspace, because nothing re-reads the seat.
func TestRemovedMemberLosesConferenceSocket(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := mkConference(t, owner, "Летучка, из которой исключают")
	mate := addMember(t, owner, s.WS)
	// Admin, so the seat carries moderation: the sharpest form of the leak.
	owner.expect(t, owner.patch("/workspaces/"+s.WS+"/members/"+mate.UserID,
		map[string]any{"role": "admin"}), http.StatusOK)

	stayer, status := dialConfWS(t, owner.token, confID)
	if stayer == nil {
		t.Fatalf("owner handshake = %d", status)
	}
	defer stayer.Close()
	leaver, status := dialConfWS(t, mate.token, confID)
	if leaver == nil {
		t.Fatalf("member handshake = %d", status)
	}
	defer leaver.Close()
	welcome := awaitConfFrame(t, leaver, "welcome", 5*time.Second)
	if welcome == nil {
		t.Fatal("no welcome for the member")
	}
	leaverID, _ := welcome["user_id"].(string)
	if awaitState(t, stayer, 5*time.Second, func(m map[string]any) bool {
		_, ok := people(m)[leaverID]
		return ok
	}) == nil {
		t.Fatal("the member never appeared in the room snapshot")
	}

	if r := owner.del("/workspaces/" + s.WS + "/members/" + mate.UserID); r.Status != http.StatusNoContent {
		t.Fatalf("remove member: status %d\n%s", r.Status, r.Body)
	}

	if !awaitSocketClose(leaver, 5*time.Second) {
		t.Fatal("the conference socket of a removed member was never closed")
	}
	gone := awaitState(t, stayer, 5*time.Second, func(m map[string]any) bool {
		_, ok := people(m)[leaverID]
		return !ok
	})
	if gone == nil {
		t.Fatal("the removed member kept their seat in the call")
	}
	if conn, status := dialConfWS(t, mate.token, confID); status != http.StatusForbidden {
		if conn != nil {
			conn.Close()
		}
		t.Fatalf("a removed member re-dialled the conference: status %d, want 403", status)
	}
}
