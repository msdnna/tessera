// A recurrence must carry the task's stage and its Eisenhower quadrant into the
// next occurrence.
//
// Both live as plain columns on `tasks` — `milestone_id` (the «Этапы» screen's
// sprint pin) and `eisenhower_quadrant` (the matrix view's manual placement) —
// and both are set by a deliberate gesture: picking a stage in the task modal,
// dragging a card into a quadrant of the matrix. The two recurrence branches
// disagreed about them for the same reason they once disagreed about the
// due-notify overrides: the reschedule branch edits the task in place and
// UpdateTask names neither column, so both survive; the create_new branch builds
// the next occurrence with CreateTask, whose parameters carry neither, so the
// duplicate is born with both NULL.
//
// What that costs is not cosmetic. ListWorkspaceMilestones rolls a stage up over
// `tasks t ON t.milestone_id = m.id` — task_count, done_count and the Σ estimate
// — and the board can be scoped to one stage (?milestone=<slug>). So a repeating
// task pinned to a sprint left the sprint the first time it was ticked: its
// progress bar lost a task, the scoped board stopped showing it, and the card
// itself was still there on the unscoped board with «Не задан» where the stage
// had been. The matrix placement went the same way — a card dragged into
// "important, not urgent" fell back to the derived quadrant on every occurrence.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestRecurrenceCarriesMilestoneAndQuadrant(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	sprint := c.expect(t, c.post("/projects/"+s.Project+"/milestones", map[string]any{
		"title": "Спринт 14",
	}), http.StatusCreated)
	sprintID := sprint["id"].(string)

	// ── the create_new branch: the original stays, a duplicate carries on ──
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Месячный отчёт", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	// A checklist item of its own: the duplicate rebuilds the whole checklist, so
	// an item's stage goes the same way the parent's does.
	step := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свести цифры", "column_id": s.col(t, 0), "parent_id": origID,
	}), http.StatusCreated)
	stepID := step["id"].(string)

	c.expect(t, c.post("/tasks/"+origID+"/milestone", map[string]any{
		"milestone_id": sprintID,
	}), http.StatusNoContent)
	c.expect(t, c.post("/tasks/"+stepID+"/milestone", map[string]any{
		"milestone_id": sprintID,
	}), http.StatusNoContent)
	// "important, not urgent" — the quadrant the user dragged the card into.
	c.expect(t, c.patch("/tasks/"+origID+"/eisenhower", map[string]any{
		"quadrant": 1,
	}), http.StatusOK)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Месячный отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "monthly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Месячный отчёт")
	if got := clone["milestone_id"]; got != sprintID {
		t.Fatalf("the duplicate left the stage: milestone_id=%v, want %s", got, sprintID)
	}
	if got := clone["eisenhower_quadrant"]; got != float64(1) {
		t.Fatalf("the duplicate lost the matrix quadrant: %v, want 1", got)
	}
	cloneStep := subtaskByTitle(t, clone, "Свести цифры")
	stepDetail := c.expect(t, c.get("/tasks/"+cloneStep["id"].(string)), http.StatusOK)
	if got := stepDetail["milestone_id"]; got != sprintID {
		t.Fatalf("the duplicate's checklist item left the stage: milestone_id=%v", got)
	}

	// The spent occurrence keeps its own pin — the duplicate copied it, it did not
	// take it.
	first := c.expect(t, c.get("/tasks/"+origID), http.StatusOK)
	if got := first["milestone_id"]; got != sprintID {
		t.Fatalf("the first occurrence lost its stage to the duplicate: %v", got)
	}

	// ── the reschedule branch: the same task moves to the next occurrence ──
	// It never rebuilt anything, so it is the control: what a person pinned
	// survives a repeat, which is what the duplicate has to match.
	daily := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	dailyID := daily["id"].(string)
	c.expect(t, c.post("/tasks/"+dailyID+"/milestone", map[string]any{
		"milestone_id": sprintID,
	}), http.StatusNoContent)
	c.expect(t, c.patch("/tasks/"+dailyID+"/eisenhower", map[string]any{
		"quadrant": 3,
	}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+dailyID, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)
	next := c.expect(t, c.get("/tasks/"+dailyID), http.StatusOK)
	if got := next["milestone_id"]; got != sprintID {
		t.Fatalf("a reschedule unpinned the task from its stage: %v", got)
	}
	if got := next["eisenhower_quadrant"]; got != float64(3) {
		t.Fatalf("a reschedule reset the matrix quadrant: %v", got)
	}

	// A task nobody pinned stays unpinned: the carry copies what was set, it does
	// not invent a stage or a quadrant for the duplicate.
	plain := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Полив цветов", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	plainID := plain["id"].(string)
	c.expect(t, c.patch("/tasks/"+plainID, map[string]any{
		"title": "Полив цветов", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)
	plainClone := findClone(t, c, s.Board, plainID, "Полив цветов")
	if got := plainClone["milestone_id"]; got != nil {
		t.Fatalf("the duplicate invented a stage: %v", got)
	}
	if got := plainClone["eisenhower_quadrant"]; got != nil {
		t.Fatalf("the duplicate invented a matrix quadrant: %v", got)
	}
}
