// A recurrence that fires "on schedule" is the one the clock drives: the editor's
// own wording for the trigger is «По расписанию», and its point is that the next
// occurrence arrives whether or not anyone closed the current one. Ticking the
// box on such a task therefore has to be an ordinary act — the occurrence is
// done, the series is not.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// Closing a schedule-triggered occurrence does not end the series.
//
// ListScheduleRecurDue selects the tasks the worker advances, and it filtered
// `completed_at IS NULL` — the filter every other sweep here carries, and the
// only one of its three conditions that does not hold for this trigger. Nothing
// else picks the task back up: recurAdvance runs on completion with
// TriggerComplete, and a schedule rule's trigger doesn't match, so it is a no-op.
// So one tick of the checkbox (or one drag into «Готово», which completes the
// card the same way) left the task completed forever, invisible to the worker,
// with its rule still on it — the modal kept saying «Еженедельно / По
// расписанию» and the card was never coming back.
//
// Both closing paths are checked because they are separate code: the checkbox
// goes through applyTaskPatch, the done column through applyMove's auto-toggle.
func TestScheduleRecurrenceSurvivesBeingTickedDone(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo, done := s.col(t, 0), s.col(t, 3)
	// Comfortably in the past, so the worker considers the occurrence elapsed.
	past := time.Now().AddDate(0, 0, -30).UTC().Truncate(time.Second)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }
	weekly := map[string]any{"freq": "weekly", "interval": 1, "trigger": "schedule"}

	// ── the checkbox ────────────────────────────────────────────────
	ticked := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo, "due_date": rfc(past),
	}), http.StatusCreated)
	tickedID := ticked["id"].(string)
	// Rule and closure in one write, so no worker tick can land between them and
	// advance the task while it is still open.
	c.expect(t, c.patch("/tasks/"+tickedID, map[string]any{
		"due_date": rfc(past), "recurrence": weekly, "completed": true,
	}), http.StatusOK)

	// ── the done column ─────────────────────────────────────────────
	dragged := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Списание расходов", "column_id": todo, "due_date": rfc(past),
	}), http.StatusCreated)
	draggedID := dragged["id"].(string)
	c.expect(t, c.patch("/tasks/"+draggedID, map[string]any{
		"due_date": rfc(past), "recurrence": weekly,
	}), http.StatusOK)
	moved := c.expect(t, c.patch("/tasks/"+draggedID+"/move",
		map[string]any{"column_id": done}), http.StatusOK)
	if moved["completed_at"] == nil {
		t.Fatalf("fixture is wrong: the done column did not complete the card: %v", moved)
	}

	if !testAPI.AdvanceRecurrencesOnce(context.Background()) {
		t.Fatal("the recurrence sweep did not run (advisory lock held elsewhere)")
	}

	for _, tc := range []struct{ id, how string }{
		{tickedID, "ticked done in the modal"},
		{draggedID, "dragged into «Готово»"},
	} {
		got := c.expect(t, c.get("/tasks/"+tc.id), http.StatusOK)
		if got["completed_at"] != nil {
			t.Fatalf("task %s: the schedule worker left it closed — the series is dead, "+
				"yet the rule is still on the card: completed_at=%v recurrence=%v",
				tc.how, got["completed_at"], got["recurrence"])
		}
		next, err := time.Parse(time.RFC3339, got["due_date"].(string))
		if err != nil {
			t.Fatalf("task %s: due date %v: %v", tc.how, got["due_date"], err)
		}
		if !next.After(time.Now()) {
			t.Fatalf("task %s: due date stayed in the past (%s) — no next occurrence", tc.how, next)
		}
		if got["recurrence"] == nil {
			t.Fatalf("task %s: the sweep dropped the rule instead of advancing it", tc.how)
		}
	}
}
