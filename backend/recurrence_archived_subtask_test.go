// A recurrence must not resurrect a checklist item that was put in the archive.
//
// Archiving one item of a repeating task is the ordinary way to say "this step is
// no longer part of the routine" while keeping the rest — it is a gesture with its
// own undo (the item stands in the board's archive as an entry of its own, because
// its parent is still active). The two recurrence branches disagreed about it: the
// reschedule branch edits the task in place and never looks at the checklist's
// archived state, while the create_new branch rebuilds the checklist from
// ListSubtasks, which does not filter archived_at, and hands every row to
// CreateTask — which is born active. So the archived item came back alive in the
// next occurrence, with its tags and assignees, and again in the one after that,
// while the archived original stayed in the archive: one deletion, endless copies.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestRecurrenceSkipsArchivedSubtask(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	// ── the create_new branch: the original stays, a duplicate carries on ──
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	kept := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свести цифры", "column_id": s.col(t, 0), "parent_id": origID,
	}), http.StatusCreated)
	dropped := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Снять слепок вручную", "column_id": s.col(t, 0), "parent_id": origID,
	}), http.StatusCreated)
	droppedID := dropped["id"].(string)

	// The item carries a tag and an assignee: the clone copies both, so a
	// resurrected row is not just a bare title reappearing.
	tag := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "рутина", "color": "#3b82f6"}), http.StatusCreated)
	c.expect(t, c.post("/tasks/"+droppedID+"/tags",
		map[string]any{"tag_id": tag["id"]}), http.StatusNoContent)

	c.expect(t, c.patch("/tasks/"+droppedID+"/archive", nil), http.StatusNoContent)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Еженедельный отчёт")
	if !hasSubtaskTitled(clone, "Свести цифры") {
		t.Fatalf("the duplicate lost a live checklist item: %v", clone["subtasks"])
	}
	if hasSubtaskTitled(clone, "Снять слепок вручную") {
		t.Fatalf("the duplicate resurrected an archived checklist item: %v", clone["subtasks"])
	}
	// The archived original is untouched by the recurrence — still one entry in the
	// board's archive, not zero (it was not restored) and not two (it was not copied
	// into the archive as well).
	if got := countArchivedTitled(t, c, s.Board, "Снять слепок вручную"); got != 1 {
		t.Fatalf("archive holds %d copies of the archived item, want 1", got)
	}
	// The live item was copied, not moved: the first occurrence keeps its checklist.
	first := c.expect(t, c.get("/tasks/"+origID), http.StatusOK)
	if !hasSubtaskTitled(first, "Свести цифры") {
		t.Fatalf("the first occurrence lost its checklist: %v", first["subtasks"])
	}
	if keptID := kept["id"].(string); subtaskByTitle(t, first, "Свести цифры")["id"] != keptID {
		t.Fatalf("the duplicate took the original's subtask instead of copying it")
	}

	// ── the reschedule branch: the same task moves to the next occurrence ──
	// It never rebuilt the checklist, so it is the control: an archived item stays
	// archived across a repeat, which is what the duplicate has to match.
	daily := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	dailyID := daily["id"].(string)
	skip := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Отменённый пункт", "column_id": s.col(t, 0), "parent_id": dailyID,
	}), http.StatusCreated)
	c.expect(t, c.patch("/tasks/"+skip["id"].(string)+"/archive", nil), http.StatusNoContent)

	c.expect(t, c.patch("/tasks/"+dailyID, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	if got := countArchivedTitled(t, c, s.Board, "Отменённый пункт"); got != 1 {
		t.Fatalf("after a reschedule the archive holds %d copies of the archived item, want 1", got)
	}
	for _, m := range c.get("/boards/" + s.Board + "/tasks").listBody(t) {
		if m["title"] == "Отменённый пункт" {
			t.Fatalf("a reschedule put the archived item back on the board: %v", m)
		}
	}
}

// hasSubtaskTitled reports whether a task detail's checklist holds an item by title.
func hasSubtaskTitled(detail map[string]any, title string) bool {
	subs, _ := detail["subtasks"].([]any)
	for _, s := range subs {
		if m, ok := s.(map[string]any); ok && m["title"] == title {
			return true
		}
	}
	return false
}

// countArchivedTitled counts the entries of a board's archive with a given title.
func countArchivedTitled(t *testing.T, c *client, boardID, title string) int {
	t.Helper()
	n := 0
	for _, m := range c.get("/boards/" + boardID + "/archive").listBody(t) {
		if m["title"] == title {
			n++
		}
	}
	return n
}
