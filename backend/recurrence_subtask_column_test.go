// A recurrence hands back a checklist that is unticked *and* off the done column.
// The two are one fact on this board: a card enters the done column and the server
// completes it, it leaves and the server reopens it (MoveTask's auto-toggle). A
// subtask that was closed by dragging it into "Готово" therefore carries its state
// in two places, and reopening only one of them leaves a card the board draws under
// "Готово" with an empty checkbox.
package main

import (
	"net/http"
	"testing"
	"time"
)

// A checklist item closed by the done column comes back out of it when the parent
// recurs.
//
// The parent already does: applyRecur moves it to the rule's target column (or the
// board's first) precisely so the next occurrence starts where work starts. Its
// items are tasks in their own right — own card, own column — and ReopenSubtasks
// only cleared completed_at, so an item dragged into "Готово" stayed there while
// its checkbox emptied. The create_new branch never had the problem: it builds the
// duplicate's checklist in the target column. Both branches are checked here
// because they rebuild the checklist in two separate places.
func TestRecurrenceTakesSubtasksOutOfDoneColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo, doing, done := s.col(t, 0), s.col(t, 1), s.col(t, 3)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	// ── the reschedule branch: the task itself moves to the next occurrence ──
	parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	parentID := parent["id"].(string)

	closed := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Собрать метрики", "column_id": todo, "parent_id": parentID,
	}), http.StatusCreated)
	closedID := closed["id"].(string)
	// An item the user left in mid-flight: it is not in the done column, so the
	// recurrence has no business relocating it.
	c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Проверить дашборд", "column_id": doing, "parent_id": parentID,
	}), http.StatusCreated)

	// Closing the item the board's own way: drag it into "Готово".
	moved := c.expect(t, c.patch("/tasks/"+closedID+"/move",
		map[string]any{"column_id": done}), http.StatusOK)
	if moved["completed_at"] == nil {
		t.Fatalf("the done column did not complete the subtask — fixture is wrong: %v", moved)
	}

	c.expect(t, c.patch("/tasks/"+parentID, map[string]any{
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	advanced := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
	if advanced["column_id"] == done {
		t.Fatalf("fixture: the recurring parent stayed in the done column")
	}
	reopened := subtaskByTitle(t, advanced, "Собрать метрики")
	if reopened["completed_at"] != nil {
		t.Fatalf("recurrence left the checklist ticked: %v", reopened["completed_at"])
	}
	if reopened["column_id"] == done {
		t.Fatalf("reopened subtask is still parked in the done column: an empty checkbox under «Готово»")
	}
	if got, want := reopened["column_id"], advanced["column_id"]; got != want {
		t.Fatalf("reopened subtask landed in column %v, want the parent's %v", got, want)
	}
	// Untouched: it was never in the done column, so its own column is its own business.
	if still := subtaskByTitle(t, advanced, "Проверить дашборд"); still["column_id"] != doing {
		t.Fatalf("recurrence relocated a subtask that was not in the done column: %v", still["column_id"])
	}

	// ── the create_new branch: the duplicate's checklist starts in the target ──
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	origSub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свести цифры", "column_id": todo, "parent_id": origID,
	}), http.StatusCreated)
	c.expect(t, c.patch("/tasks/"+origSub["id"].(string)+"/move",
		map[string]any{"column_id": done}), http.StatusOK)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Еженедельный отчёт")
	cloned := subtaskByTitle(t, clone, "Свести цифры")
	if cloned["column_id"] == done {
		t.Fatalf("the duplicate's checklist was born in the done column: %v", cloned["column_id"])
	}
	if got, want := cloned["column_id"], clone["column_id"]; got != want {
		t.Fatalf("cloned subtask landed in column %v, want the clone's %v", got, want)
	}
}
