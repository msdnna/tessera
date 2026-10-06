// `column.deleted` has to name its board (#2932).
//
// The event is workspace-scoped like every other, so it reaches every board a
// colleague has open, not just the one that lost a column. On the web that
// deletion is the most expensive classification there is — `column-purge`
// re-reads the columns, the task lists AND the board record, because a deleted
// column takes its cards with it and may have been the board's done column.
//
// It used to travel as a bare `{id}`, which left the reader nothing to compare:
// every open board in the workspace had to run the purge on the chance that the
// column was its own. With the board in the payload, only the board that lost it
// does any work (frontend utils/boardEvents classifyEvent).
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestDeleteColumnBroadcastsItsBoard(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)

	created := owner.expect(t, owner.post("/boards/"+st.Board+"/columns",
		map[string]any{"name": "Лишняя", "color": ""}), http.StatusCreated)
	colID := created["id"].(string)

	conn, status := dialWS(t, owner.token, false)
	if conn == nil {
		t.Fatalf("owner handshake failed (status %d)", status)
	}
	defer conn.Close()

	if r := owner.del("/columns/" + colID); r.Status != http.StatusNoContent {
		t.Fatalf("delete column status %d, want 204\n%s", r.Status, r.Body)
	}

	ev := awaitWSEvent(t, conn, "column.deleted", 5*time.Second)
	if ev == nil {
		t.Fatal("deleting a column broadcast no column.deleted event")
	}
	data, _ := ev["data"].(map[string]any)
	if data["id"] != colID {
		t.Fatalf("column.deleted named column %v, want %s", data["id"], colID)
	}
	if data["board_id"] != st.Board {
		t.Fatalf("column.deleted carried board %v, want %s — without it every open board in the workspace has to purge and re-read itself",
			data["board_id"], st.Board)
	}
}
