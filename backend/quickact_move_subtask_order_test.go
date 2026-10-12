package main

import (
	"net/http"
	"testing"
)

// `/move`, `/close` and `/reopen` are a column change typed instead of dragged,
// so they name no neighbours — and the server picks the slot. For a top-level
// card that means the end of the destination column; for a SUBTASK it means the
// row it already stands on, because a subtask is numbered among its parent's
// children and changing column does not take it out of that list.
//
// The quick-action path had its own copy of the append, through the rule
// CreateTask uses: a NEW subtask goes to the end of the parent's list. A subtask
// that merely changed column went there too, so `/close` on the first of three
// children silently dropped it to the bottom of the parent's subtask list —
// while the very same move from the task modal (which sends the sibling
// neighbours of the row the subtask stands on) left the order alone.

// mkSubtask creates a child under [parent] in [columnID].
func mkSubtask(t *testing.T, c *client, boardID, columnID, parent, title string) map[string]any {
	t.Helper()
	return c.expect(t, c.post("/boards/"+boardID+"/tasks",
		map[string]any{"title": title, "column_id": columnID, "parent_id": parent}), http.StatusCreated)
}

// `/move` on a subtask changes its column and leaves the parent's order alone.
func TestQuickActionMoveSubtaskKeepsSiblingOrder(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo, doing := s.col(t, 0), s.col(t, 1)

	parent := mkTask(t, c, s.Board, todo, "Родитель")["id"].(string)
	first := mkSubtask(t, c, s.Board, todo, parent, "Первая подзадача")
	firstID, firstPos := first["id"].(string), first["position"].(float64)
	second := mkSubtask(t, c, s.Board, todo, parent, "Вторая подзадача")["id"].(string)
	third := mkSubtask(t, c, s.Board, todo, parent, "Третья подзадача")["id"].(string)

	r := c.post("/tasks/"+firstID+"/comments", map[string]any{"body": "/move В процессе"})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
	}
	if applied, failed := commandSummary(t, r); len(applied) != 1 || applied[0] != "move" || len(failed) != 0 {
		t.Fatalf("summary applied=%v failed=%v, want [move] / []", applied, failed)
	}

	after := c.expect(t, c.get("/tasks/"+firstID), http.StatusOK)
	if after["column_id"] != doing {
		t.Fatalf("subtask landed in column %v, want %s", after["column_id"], doing)
	}
	// The reorder's fingerprint: the moved child's position was rewritten to
	// «behind the last sibling» instead of staying where it was.
	if got := after["position"]; got != firstPos {
		t.Fatalf("subtask position after /move = %v, want %v (unchanged)", got, firstPos)
	}
	ids := filterIDs(idsIn(t, c, "/boards/"+s.Board+"/subtasks"), firstID, second, third)
	assertSameOrder(t, "parent's children after /move on the first one", ids,
		[]string{firstID, second, third})
}

// Same for `/close`, which resolves the board's done column instead of a typed
// name — the slot is picked in one place for all three commands.
func TestQuickActionCloseSubtaskKeepsSiblingOrder(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)
	done := s.col(t, len(s.Columns)-1) // the board's rightmost column closes tasks

	parent := mkTask(t, c, s.Board, todo, "Родитель под /close")["id"].(string)
	first := mkSubtask(t, c, s.Board, todo, parent, "Первая подзадача")
	firstID, firstPos := first["id"].(string), first["position"].(float64)
	second := mkSubtask(t, c, s.Board, todo, parent, "Вторая подзадача")["id"].(string)

	r := c.post("/tasks/"+firstID+"/comments", map[string]any{"body": "/close"})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
	}
	after := c.expect(t, c.get("/tasks/"+firstID), http.StatusOK)
	if after["column_id"] != done {
		t.Fatalf("subtask landed in column %v, want %s (done)", after["column_id"], done)
	}
	if got := after["position"]; got != firstPos {
		t.Fatalf("subtask position after /close = %v, want %v (unchanged)", got, firstPos)
	}
	ids := filterIDs(idsIn(t, c, "/boards/"+s.Board+"/subtasks"), firstID, second)
	assertSameOrder(t, "parent's children after /close on the first one", ids,
		[]string{firstID, second})
}

// The top-level half of the same rule is untouched: a card moved by `/move`
// still appends to the end of the destination column.
func TestQuickActionMoveTaskAppendsToColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo, doing := s.col(t, 0), s.col(t, 1)

	standing := mkTask(t, c, s.Board, doing, "Уже в колонке")["id"].(string)
	mover := mkTask(t, c, s.Board, todo, "Переносимая")["id"].(string)

	r := c.post("/tasks/"+mover+"/comments", map[string]any{"body": "/move В процессе"})
	if r.Status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", r.Status, r.Body)
	}
	after := c.expect(t, c.get("/tasks/"+mover), http.StatusOK)
	if after["column_id"] != doing {
		t.Fatalf("task landed in column %v, want %s", after["column_id"], doing)
	}
	ids := filterIDs(idsIn(t, c, "/boards/"+s.Board+"/tasks"), standing, mover)
	assertSameOrder(t, "destination column after /move", ids, []string{standing, mover})
}
