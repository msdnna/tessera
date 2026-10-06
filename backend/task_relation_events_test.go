// Realtime events for task relations (#2932).
//
// The Gantt view draws a blocking relation as an arrow between two bars, and it
// holds that edge list in its own state — a separate fetch from the board's task
// lists. So a relation added or dropped anywhere else (the task modal, a second
// browser, a quick action) only reaches an open chart if the socket says so.
//
// Adding one at least broadcast `task.updated` for both endpoints; DELETE
// /relations/:id broadcast nothing at all, which left every other client with no
// reason to re-read. These pin a relation-specific event on both edges of the
// pair, carrying the board of each endpoint so a board can tell whether the
// change concerns it.
package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// awaitWSEvent reads workspace events until one of the wanted type shows up or
// the budget runs out. A relation change rides in behind whatever the fixture
// itself broadcast (task.created, …), so insisting on the next frame would be
// flaky by construction.
func awaitWSEvent(t *testing.T, conn *websocket.Conn, want string, budget time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		ev := readEvent(t, conn, time.Until(deadline))
		if ev == nil {
			return nil
		}
		if ev["type"] == want {
			return ev
		}
	}
	return nil
}

func TestTaskRelationBroadcastsOnAddAndDelete(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)
	colID := st.col(t, 0)

	blocker := mkTask(t, owner, st.Board, colID, "Блокер")
	blocked := mkTask(t, owner, st.Board, colID, "Заблокированная")
	blockerID := blocker["id"].(string)
	blockedNum := blocked["number"]

	conn, status := dialWS(t, owner.token, false)
	if conn == nil {
		t.Fatalf("owner handshake failed (status %d)", status)
	}
	defer conn.Close()

	owner.expect(t, owner.post("/tasks/"+blockerID+"/relations",
		map[string]any{"number": blockedNum, "kind": "blocks"}), http.StatusCreated)

	ev := awaitWSEvent(t, conn, "task.relation", 5*time.Second)
	if ev == nil {
		t.Fatal("adding a relation broadcast no task.relation event — an open Gantt never learns about the new arrow")
	}
	data, _ := ev["data"].(map[string]any)
	if data["board_id"] != st.Board || data["related_board_id"] != st.Board {
		t.Fatalf("task.relation carried boards %v/%v, want both %s — a board cannot tell whether the change concerns it",
			data["board_id"], data["related_board_id"], st.Board)
	}

	rels := owner.get("/tasks/" + blockerID + "/relations").listBody(t)
	if len(rels) != 1 {
		t.Fatalf("relation list has %d rows, want 1", len(rels))
	}
	relID := rels[0]["id"].(string)

	if r := owner.del("/relations/" + relID); r.Status != http.StatusNoContent {
		t.Fatalf("delete relation status %d, want 204\n%s", r.Status, r.Body)
	}
	if ev := awaitWSEvent(t, conn, "task.relation", 5*time.Second); ev == nil {
		t.Fatal("deleting a relation broadcast nothing — the arrow stays on every other client's Gantt until a reload")
	}
}
