// The recurrence editor offers three events — «При завершении», «При переходе в
// колонку» and «По расписанию» — and the middle one is the only one no test ever
// exercised end to end: recur_test.go checks that the rule parses, and nothing
// checked that dragging a card into the chosen column actually advances it.
//
// The column trigger is also the one event that shares its code path with the
// board's done-column auto-complete (both live in applyMove, one after the
// other), so the combination people reach for first — «повторять, когда карточка
// уезжает в Готово» — runs two mechanisms over one move.
package main

import (
	"net/http"
	"testing"
	"time"
)

// Dragging a card into the rule's trigger column advances it; dragging it
// anywhere else does not.
//
// Both branches are checked because they are separate code: the reschedule
// branch edits the task in place, the create_new branch leaves the original as a
// spent occurrence and builds a duplicate.
func TestColumnTriggerRecurrenceAdvancesOnTheMove(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo, doing, review := s.col(t, 0), s.col(t, 1), s.col(t, 2)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }
	dueOf := func(m map[string]any) time.Time {
		t.Helper()
		got, err := time.Parse(time.RFC3339, m["due_date"].(string))
		if err != nil {
			t.Fatalf("due date %v: %v", m["due_date"], err)
		}
		return got.UTC()
	}

	// ── the reschedule branch ───────────────────────────────────────
	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Разбор входящих", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	taskID := task["id"].(string)
	c.expect(t, c.patch("/tasks/"+taskID, map[string]any{
		"recurrence": map[string]any{
			"freq": "daily", "interval": 1,
			"trigger": "column", "trigger_column": review, "target_column": doing,
		},
	}), http.StatusOK)

	// Negative control: a move into any other column is an ordinary move.
	idle := c.expect(t, c.patch("/tasks/"+taskID+"/move",
		map[string]any{"column_id": doing}), http.StatusOK)
	if got := dueOf(idle); !got.Equal(due) {
		t.Fatalf("a move into a column that is not the trigger advanced the task: due %s, want %s", got, due)
	}
	if idle["column_id"] != doing {
		t.Fatalf("the control move did not land: column %v, want %v", idle["column_id"], doing)
	}

	moved := c.expect(t, c.patch("/tasks/"+taskID+"/move",
		map[string]any{"column_id": review}), http.StatusOK)
	if got, want := dueOf(moved), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("the trigger column did not advance the task: due %s, want %s", got, want)
	}
	// The card does not stay where the drag put it: the rule says where the next
	// occurrence starts, and that is the whole point of «Переносить в».
	if got := moved["column_id"]; got != doing {
		t.Fatalf("the next occurrence landed in column %v, want the rule's target %v", got, doing)
	}
	if moved["recurrence"] == nil {
		t.Fatal("the rule was dropped instead of carried to the next occurrence")
	}

	// ── the create_new branch ───────────────────────────────────────
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Дежурство", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)
	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"recurrence": map[string]any{
			"freq": "weekly", "interval": 1, "create_new": true,
			"trigger": "column", "trigger_column": review, "target_column": doing,
		},
	}), http.StatusOK)
	spent := c.expect(t, c.patch("/tasks/"+origID+"/move",
		map[string]any{"column_id": review}), http.StatusOK)
	if spent["recurrence"] != nil {
		t.Fatal("the spent occurrence kept the rule — it would fire again on the next drag")
	}
	if got := dueOf(spent); !got.Equal(due) {
		t.Fatalf("the spent occurrence was re-dated: due %s, want the original %s", got, due)
	}

	clone := findClone(t, c, s.Board, origID, "Дежурство")
	if got, want := dueOf(clone), due.AddDate(0, 0, 7); !got.Equal(want) {
		t.Fatalf("the duplicate's due date is %s, want %s", got, want)
	}
	if clone["column_id"] != doing {
		t.Fatalf("the duplicate was born in column %v, want the rule's target %v", clone["column_id"], doing)
	}
	if clone["recurrence"] == nil {
		t.Fatal("the duplicate carries no rule — the series ends after one drag")
	}
	if clone["completed_at"] != nil {
		t.Fatalf("the duplicate was born completed: %v", clone["completed_at"])
	}
}

// The trigger column being the board's done column is the setup the editor
// invites — «повторять, когда карточка уезжает в Готово» — and it is the one
// move that runs both mechanisms in applyMove: the auto-complete stamps
// completed_at, then the column trigger advances the task.
//
// The series has to win: an occurrence that is immediately replaced by the next
// one is not a closed card, the same way a complete-triggered card bounces
// straight back out of the done column.
func TestColumnTriggerIntoDoneColumnKeepsTheSeriesOpen(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo, done := s.col(t, 0), s.col(t, 3)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Полив цветов", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	taskID := task["id"].(string)
	c.expect(t, c.patch("/tasks/"+taskID, map[string]any{
		"recurrence": map[string]any{
			"freq": "daily", "interval": 1,
			"trigger": "column", "trigger_column": done, "target_column": todo,
		},
	}), http.StatusOK)

	moved := c.expect(t, c.patch("/tasks/"+taskID+"/move",
		map[string]any{"column_id": done}), http.StatusOK)

	got, err := time.Parse(time.RFC3339, moved["due_date"].(string))
	if err != nil {
		t.Fatalf("due date %v: %v", moved["due_date"], err)
	}
	if want := due.AddDate(0, 0, 1); !got.UTC().Equal(want) {
		t.Fatalf("the done column did not advance the series: due %s, want %s", got.UTC(), want)
	}
	if moved["completed_at"] != nil {
		t.Fatalf("the next occurrence is closed: the auto-complete outlived the recurrence (%v)", moved["completed_at"])
	}
	if moved["column_id"] != todo {
		t.Fatalf("the next occurrence stayed in column %v, want the rule's target %v", moved["column_id"], todo)
	}
	if moved["recurrence"] == nil {
		t.Fatal("the rule was dropped — the series ends on its first turn")
	}
}
