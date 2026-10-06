// A recurrence carries the whole checklist forward, not just the parent's window.
// Both branches are covered here — the reschedule of the same task and the
// create_new duplicate — because they rebuild the checklist in two separate
// places and each one dropped a different half of it.
package main

import (
	"net/http"
	"testing"
	"time"
)

// Subtask dates must step by the same amount the parent's due date did.
//
// A subtask is a task in its own right: it shows on the board, in "my tasks" and
// on the Gantt/timeline as a bar of its own, and it goes red once its due date
// passes. Advancing only the parent left every checklist item pinned to the very
// first occurrence — the second run of a daily standup is born with all of its
// items already overdue, drawn a day behind the parent they belong to, and it only
// gets worse each cycle. shiftStart already states the rule for the parent ("the
// window has to move whole, not stretch"); the children live under the same rule.
func TestRecurrenceCarriesSubtaskWindow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	// ── the reschedule branch: the task itself moves to the next occurrence ──
	parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": s.col(t, 0),
		"start_date": rfc(start), "due_date": rfc(due),
	}), http.StatusCreated)
	parentID := parent["id"].(string)

	c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Собрать метрики", "column_id": s.col(t, 0), "parent_id": parentID,
		"start_date": rfc(start), "due_date": rfc(due), "estimate": 1.5,
	}), http.StatusCreated)
	// A checklist item nobody scheduled must not grow dates out of the recurrence.
	c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Без срока", "column_id": s.col(t, 0), "parent_id": parentID,
	}), http.StatusCreated)

	c.expect(t, c.patch("/tasks/"+parentID, map[string]any{
		"title": "Ежедневный статус", "start_date": rfc(start), "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	advanced := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
	moved := subtaskByTitle(t, advanced, "Собрать метрики")
	if got, want := parseTS(t, moved["due_date"]), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("rescheduled subtask due = %v, want %v (the checklist moves with its parent)", got, want)
	}
	if moved["start_date"] == nil {
		t.Fatalf("rescheduled subtask lost its start_date: %v", moved)
	}
	if got, want := parseTS(t, moved["start_date"]), start.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("rescheduled subtask start = %v, want %v", got, want)
	}
	if bare := subtaskByTitle(t, advanced, "Без срока"); bare["due_date"] != nil || bare["start_date"] != nil {
		t.Fatalf("recurrence invented dates for an undated subtask: %v", bare)
	}

	// ── the create_new branch: the original stays, a duplicate carries on ──
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": s.col(t, 0),
		"start_date": rfc(start), "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	origSub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свести цифры", "column_id": s.col(t, 0), "parent_id": origID,
		"start_date": rfc(start), "due_date": rfc(due), "estimate": 2,
	}), http.StatusCreated)
	origSubID := origSub["id"].(string)

	tag := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "отчётность", "color": "#3b82f6"}), http.StatusCreated)
	c.expect(t, c.post("/tasks/"+origSubID+"/tags",
		map[string]any{"tag_id": tag["id"]}), http.StatusNoContent)
	c.expect(t, c.post("/tasks/"+origSubID+"/assignees",
		map[string]any{"user_id": c.UserID}), http.StatusNoContent)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "start_date": rfc(start), "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Еженедельный отчёт")
	cloned := subtaskByTitle(t, clone, "Свести цифры")
	if got, want := parseTS(t, cloned["due_date"]), due.AddDate(0, 0, 7); !got.Equal(want) {
		t.Fatalf("cloned subtask due = %v, want %v (the duplicate is the next occurrence, not a copy of the first)", got, want)
	}
	if cloned["start_date"] == nil {
		t.Fatalf("cloned subtask lost its start_date: %v", cloned)
	}
	if got, want := parseTS(t, cloned["start_date"]), start.AddDate(0, 0, 7); !got.Equal(want) {
		t.Fatalf("cloned subtask start = %v, want %v", got, want)
	}
	// The same rule rescheduled in place keeps the item's estimate, tags and
	// assignees; the duplicate must not quietly hand back a barer checklist.
	if cloned["estimate"] == nil {
		t.Fatalf("cloned subtask lost its estimate: %v", cloned)
	}
	if got, ok := cloned["estimate"].(float64); !ok || got != 2 {
		t.Fatalf("cloned subtask estimate = %v, want 2", cloned["estimate"])
	}
	if !containsID(cloned["assignee_ids"], c.UserID) {
		t.Fatalf("cloned subtask lost its assignee: %v", cloned["assignee_ids"])
	}
	if !containsID(cloned["tag_ids"], tag["id"].(string)) {
		t.Fatalf("cloned subtask lost its tag: %v", cloned["tag_ids"])
	}
	// The clone's checklist starts unticked, and it is a copy — not a move.
	if cloned["completed_at"] != nil {
		t.Fatalf("cloned subtask arrived already completed: %v", cloned["completed_at"])
	}
	if cloned["id"] == origSubID {
		t.Fatalf("the clone reused the original's subtask instead of copying it")
	}
}

// subtaskByTitle picks one item out of a task detail's checklist.
func subtaskByTitle(t *testing.T, detail map[string]any, title string) map[string]any {
	t.Helper()
	subs, _ := detail["subtasks"].([]any)
	for _, s := range subs {
		m, ok := s.(map[string]any)
		if ok && m["title"] == title {
			return m
		}
	}
	t.Fatalf("no subtask %q among %v", title, detail["subtasks"])
	return nil
}

// containsID reports whether a JSON array of uuids holds id.
func containsID(list any, id string) bool {
	arr, _ := list.([]any)
	for _, v := range arr {
		if v == id {
			return true
		}
	}
	return false
}
