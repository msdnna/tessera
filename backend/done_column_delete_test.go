// Deleting the completing column must not switch task completion off (#2932).
//
// boards.done_column_id is an FK with ON DELETE SET NULL (migration 0007), from
// the days when NULL meant "fall back to the rightmost column". Since #2588 NULL
// means the board has NO completing column — doneColumnID has no fallback left.
// So dropping the done column quietly turned auto-completion off for good: cards
// dragged into the last column never got completed_at, and nothing said why.
// Migration 0046 already repaired that state once for legacy NULL boards by
// pinning them to their rightmost column; DeleteColumn now does the same repair
// in the transaction that opens the hole.
package main

import (
	"net/http"
	"testing"
)

// boardDone reads a board's configured done column ("" when it has none).
func boardDone(t *testing.T, c *client, boardID string) string {
	t.Helper()
	b := c.expect(t, c.get("/boards/"+boardID), http.StatusOK)
	if id, ok := b["done_column_id"].(string); ok {
		return id
	}
	return ""
}

func TestDeleteDoneColumnRepointsToRightmost(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)

	done := boardDone(t, owner, st.Board)
	if done == "" {
		t.Fatal("a seeded board should come with a done column")
	}
	if want := st.col(t, len(st.Columns)-1); done != want {
		t.Fatalf("seeded done column %s, want the rightmost %s", done, want)
	}

	if r := owner.del("/columns/" + done); r.Status != http.StatusNoContent {
		t.Fatalf("delete done column status %d, want 204\n%s", r.Status, r.Body)
	}

	newDone := boardDone(t, owner, st.Board)
	if newDone == "" {
		t.Fatal("board lost its done column with the deleted one — it now closes nothing, silently")
	}
	want := st.col(t, len(st.Columns)-2)
	if newDone != want {
		t.Fatalf("done column re-pointed at %s, want the new rightmost %s", newDone, want)
	}

	// The repair is only worth anything if the board actually closes tasks again.
	task := mkTask(t, owner, st.Board, st.col(t, 0), "Закрывается?")
	moved := owner.expect(t, owner.patch("/tasks/"+task["id"].(string)+"/move",
		map[string]any{"column_id": newDone}), http.StatusOK)
	if moved["completed_at"] == nil {
		t.Fatal("moving a card into the re-pointed done column did not complete it")
	}
}

// A board deliberately left without a done column must stay that way: the delete
// repairs the pointer it broke, it does not hand out a new one.
func TestDeleteColumnKeepsBoardWithoutDoneColumn(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)

	owner.expect(t, owner.patch("/boards/"+st.Board+"/done-column",
		map[string]any{"column_id": nil}), http.StatusOK)
	if got := boardDone(t, owner, st.Board); got != "" {
		t.Fatalf("done column %q survived being cleared", got)
	}

	if r := owner.del("/columns/" + st.col(t, 1)); r.Status != http.StatusNoContent {
		t.Fatalf("delete column status %d, want 204\n%s", r.Status, r.Body)
	}
	if got := boardDone(t, owner, st.Board); got != "" {
		t.Fatalf("deleting an ordinary column gave the board a done column %q it had switched off", got)
	}
}

// The last column can go too — there is nothing left to point at, and the delete
// must not fail trying.
func TestDeleteLastColumnLeavesBoardWithoutDone(t *testing.T) {
	owner := signup(t)
	st := mkStack(t, owner)

	for i := range st.Columns {
		if r := owner.del("/columns/" + st.col(t, i)); r.Status != http.StatusNoContent {
			t.Fatalf("delete column %d status %d, want 204\n%s", i, r.Status, r.Body)
		}
	}
	if got := boardDone(t, owner, st.Board); got != "" {
		t.Fatalf("empty board kept done column %q", got)
	}
	cols := owner.get("/boards/" + st.Board + "/columns").listBody(t)
	if len(cols) != 0 {
		t.Fatalf("board still has %d columns", len(cols))
	}
}
