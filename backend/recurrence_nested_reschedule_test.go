// A reschedule hands the next occurrence the WHOLE checklist — every level of it.
//
// Nesting has no depth limit (CreateTask hangs a subtask off a subtask without
// complaint, and the board draws and counts every level), so a repeating task
// decomposed two levels deep is an ordinary task. The create_new branch already
// walks the whole subtree — cloneChecklist recurses, and that was fixed on its own
// — but the reschedule branch carries the checklist with three statements that all
// name parent_id and therefore reach the DIRECT children only: ReopenSubtasks,
// LiftSubtasksFromColumn and ShiftSubtaskWindow. Everything below the first level
// was left exactly where the previous occurrence ended: still ticked, still parked
// under «Готово», and still dated for a deadline that has already passed. The
// second run of the routine is born half-done and overdue, and it gets worse every
// cycle. The two branches have to say the same thing about the same checklist.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestRecurrenceRescheduleCarriesNestedChecklist(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo, doing, done := s.col(t, 0), s.col(t, 1), s.col(t, 3)
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	mkSub := func(parentID, title, columnID string, dated bool) string {
		t.Helper()
		body := map[string]any{"title": title, "column_id": columnID, "parent_id": parentID}
		if dated {
			body["start_date"], body["due_date"] = rfc(start), rfc(due)
		}
		sub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", body), http.StatusCreated)
		return sub["id"].(string)
	}

	parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Закрытие месяца", "column_id": todo,
		"start_date": rfc(start), "due_date": rfc(due),
	}), http.StatusCreated)
	parentID := parent["id"].(string)

	// Level one — the control: this is what already worked.
	childID := mkSub(parentID, "Сверка банка", todo, true)
	// Level two — closed the board's own way, by dragging it into «Готово».
	grandID := mkSub(childID, "Выгрузить выписку", todo, true)
	// Level two, left in mid-flight and undated: the recurrence has no business
	// relocating it or inventing dates for it, at any depth.
	mkSub(childID, "Свериться с бухгалтерией", doing, false)
	// Level three, ticked by its own checkbox: depth is not a limit anywhere else.
	deepID := mkSub(grandID, "Проверить остатки", todo, true)

	closed := c.expect(t, c.patch("/tasks/"+grandID+"/move",
		map[string]any{"column_id": done}), http.StatusOK)
	if closed["completed_at"] == nil {
		t.Fatalf("the done column did not complete the nested subtask — fixture is wrong: %v", closed)
	}
	c.expect(t, c.patch("/tasks/"+deepID, map[string]any{"completed": true}), http.StatusOK)

	c.expect(t, c.patch("/tasks/"+parentID, map[string]any{
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	advanced := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
	if advanced["column_id"] == done {
		t.Fatalf("fixture: the recurring parent stayed in the done column")
	}
	if got, want := parseTS(t, advanced["due_date"]), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("fixture: parent due = %v, want %v (the recurrence did not advance)", got, want)
	}

	// ── level one: the control, green before the fix and after it ──
	child := subtaskByTitle(t, advanced, "Сверка банка")
	if got, want := parseTS(t, child["due_date"]), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("direct subtask due = %v, want %v", got, want)
	}

	// ── level two: the same checklist, one level down ──
	childDetail := c.expect(t, c.get("/tasks/"+childID), http.StatusOK)
	grand := subtaskByTitle(t, childDetail, "Выгрузить выписку")
	if grand["completed_at"] != nil {
		t.Fatalf("the next occurrence was born with a nested item already ticked: %v", grand["completed_at"])
	}
	if grand["column_id"] == done {
		t.Fatalf("nested item stayed parked under «Готово»: an empty checkbox in the done column")
	}
	if got, want := grand["column_id"], advanced["column_id"]; got != want {
		t.Fatalf("nested item landed in column %v, want the parent's %v", got, want)
	}
	if got, want := parseTS(t, grand["due_date"]), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("nested item due = %v, want %v (born overdue, pinned to the previous occurrence)", got, want)
	}
	if got, want := parseTS(t, grand["start_date"]), start.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("nested item start = %v, want %v", got, want)
	}
	// Untouched: never in the done column, never scheduled.
	parked := subtaskByTitle(t, childDetail, "Свериться с бухгалтерией")
	if parked["column_id"] != doing {
		t.Fatalf("recurrence relocated a nested subtask that was not in the done column: %v", parked["column_id"])
	}
	if parked["due_date"] != nil || parked["start_date"] != nil {
		t.Fatalf("recurrence invented dates for an undated nested subtask: %v", parked)
	}

	// ── level three: the walk does not stop at two ──
	grandDetail := c.expect(t, c.get("/tasks/"+grandID), http.StatusOK)
	deep := subtaskByTitle(t, grandDetail, "Проверить остатки")
	if deep["completed_at"] != nil {
		t.Fatalf("a third-level item stayed ticked into the next occurrence: %v", deep["completed_at"])
	}
	if got, want := parseTS(t, deep["due_date"]), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("third-level item due = %v, want %v", got, want)
	}
}
