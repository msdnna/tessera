// A project transfer carries its documents into another workspace, and the live
// rooms of those documents have to go down with the move (#2932).
//
// TestRemovedMemberLosesDocumentSocket pins the same harm through the membership
// door: ConnectDocument authorizes exactly once, at the upgrade, and nothing
// re-reads it afterwards. The transfer is the second door to it, and the quieter
// one — nobody's membership changes, the *document* changes workspace. Everyone
// sitting in these rooms is by construction a member of the source workspace
// (the target's people could not open the document a moment ago), so a room left
// up hands them the presence badges of the new owners' editors, keeps announcing
// every save of a document they are now answered 403 for, and keeps their block
// locks held — a veto on the new workspace's editing that nobody there can lift.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestTransferProjectDropsDocumentRooms(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// A second project in the source workspace is the control: it stays behind,
	// and so does the room of its document. The name carries t.Name() because a
	// project slug is globally unique and this file runs alongside the sibling
	// transfer tests.
	stayProject := mkProject(t, owner, src.WS, src.Group, "Остающийся проект "+t.Name())
	moving := mkProjectDoc(t, owner, src.WS, src.Project, "Уезжающий регламент "+t.Name())
	staying := mkProjectDoc(t, owner, src.WS, stayProject, "Остающийся регламент "+t.Name())

	// The member of the source workspace only: the boundary the transfer moves
	// passes right through them, and nothing about their membership changes.
	mate := addMember(t, owner, src.WS)

	// The owner's socket is the observer — it is also in the room, and it is
	// dropped by the transfer just like the mate's. That is intended: the drop is
	// wholesale, and the owner (a member of both sides) simply reconnects.
	watcher, status := dialDocWS(t, owner.token, moving)
	if watcher == nil {
		t.Fatalf("owner handshake = %d", status)
	}
	defer watcher.Close()
	leaver, status := dialDocWS(t, mate.token, moving)
	if leaver == nil {
		t.Fatalf("member handshake on the travelling document = %d", status)
	}
	defer leaver.Close()
	if awaitFrame(t, leaver, "welcome", 5*time.Second) == nil {
		t.Fatal("no welcome on the travelling document")
	}
	keeper, status := dialDocWS(t, mate.token, staying)
	if keeper == nil {
		t.Fatalf("member handshake on the staying document = %d", status)
	}
	defer keeper.Close()
	if awaitFrame(t, keeper, "welcome", 5*time.Second) == nil {
		t.Fatal("no welcome on the staying document")
	}

	// A held lock is what makes this more than a presence leak: without the drop
	// the block stays unavailable to the document's new workspace until the tab of
	// someone they cannot even see happens to close.
	if err := leaver.WriteJSON(map[string]any{"type": "lock", "block_id": "b1"}); err != nil {
		t.Fatalf("lock request: %v", err)
	}
	if held := awaitState(t, watcher, 5*time.Second, func(m map[string]any) bool {
		return lockNames(m)["b1"] != ""
	}); held == nil {
		t.Fatal("the member's lock never reached the room snapshot")
	}

	owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	t.Run("the room of a travelling document is dropped", func(t *testing.T) {
		if !awaitSocketClose(leaver, 5*time.Second) {
			t.Fatal("the document socket survived the transfer that took the document away")
		}
	})

	t.Run("the member left behind can no longer open it", func(t *testing.T) {
		conn, status := dialDocWS(t, mate.token, moving)
		if conn != nil {
			conn.Close()
			t.Fatalf("a non-member of the target workspace got a document socket (status %d)", status)
		}
		if status != http.StatusForbidden {
			t.Fatalf("handshake status = %d, want %d", status, http.StatusForbidden)
		}
	})

	t.Run("the lock does not travel with the document", func(t *testing.T) {
		fresh, status := dialDocWS(t, owner.token, moving)
		if fresh == nil {
			t.Fatalf("reconnect after the transfer = %d", status)
		}
		defer fresh.Close()
		if awaitFrame(t, fresh, "welcome", 5*time.Second) == nil {
			t.Fatal("no welcome on the reconnect")
		}
		state := awaitState(t, fresh, 5*time.Second, func(m map[string]any) bool {
			_, ok := m["viewers"]
			return ok
		})
		if state == nil {
			t.Fatal("no room snapshot after the reconnect")
		}
		if holder := lockNames(state)["b1"]; holder != "" {
			t.Fatalf("block b1 is still held by %q in the document's new workspace", holder)
		}
	})

	t.Run("a document of the project that stayed keeps its room", func(t *testing.T) {
		if awaitSocketClose(keeper, 2*time.Second) {
			t.Fatal("the transfer closed a room of a document it never touched")
		}
	})
}
