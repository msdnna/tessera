// A recurrence advances the task's whole planned window, not just its deadline.
// Both branches are covered here — the reschedule of the same task and the
// create_new duplicate — because they build the next occurrence in two separate
// places and only one of them being right is the bug this file is about.
package main

import (
	"net/http"
	"testing"
	"time"
)

// Recurrence must carry start_date forward by the same step as due_date.
//
// The Gantt and timeline boards draw one bar per task spanning start_date →
// due_date. A rule that advances only the deadline leaves the start pinned to the
// very first occurrence, so a daily chore's bar grows a day longer every time it
// recurs — a half-hour standup reads as a month-long effort after a month — and
// the estimate ghost bar, which anchors on the start, drifts with it.
func TestRecurrenceAdvancesStartDate(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 1, 9, 30, 0, 0, time.UTC)

	// ── the reschedule branch: the task itself moves to the next occurrence ──
	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": s.col(t, 0),
		"start_date": start.Format(time.RFC3339), "due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	id := task["id"].(string)

	adv := c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Ежедневный статус", "start_date": start.Format(time.RFC3339),
		"due_date":   due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	wantDue := due.AddDate(0, 0, 1)
	wantStart := start.AddDate(0, 0, 1)
	if got := parseTS(t, adv["due_date"]); !got.Equal(wantDue) {
		t.Fatalf("rescheduled due = %v, want %v", got, wantDue)
	}
	if adv["start_date"] == nil {
		t.Fatalf("rescheduled task lost its start_date: %v", adv)
	}
	if got := parseTS(t, adv["start_date"]); !got.Equal(wantStart) {
		t.Fatalf("rescheduled start = %v, want %v (the window must move whole, not stretch)", got, wantStart)
	}

	// ── the create_new branch: the original stays, a duplicate carries on ──
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": s.col(t, 0),
		"start_date": start.Format(time.RFC3339), "due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "start_date": start.Format(time.RFC3339),
		"due_date":   due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Еженедельный отчёт")
	wantDue = due.AddDate(0, 0, 7)
	wantStart = start.AddDate(0, 0, 7)
	if got := parseTS(t, clone["due_date"]); !got.Equal(wantDue) {
		t.Fatalf("clone due = %v, want %v", got, wantDue)
	}
	if clone["start_date"] == nil {
		t.Fatalf("clone lost its start_date: %v", clone)
	}
	if got := parseTS(t, clone["start_date"]); !got.Equal(wantStart) {
		t.Fatalf("clone start = %v, want %v (the duplicate is the next occurrence, not a copy of the first)", got, wantStart)
	}

	// A task with no start_date must not grow one out of nowhere.
	bare := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Без начала", "column_id": s.col(t, 0), "due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	bareAdv := c.expect(t, c.patch("/tasks/"+bare["id"].(string), map[string]any{
		"title": "Без начала", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)
	if bareAdv["start_date"] != nil {
		t.Fatalf("recurrence invented a start_date: %v", bareAdv["start_date"])
	}
}

// findClone returns the recurrence duplicate of origID on the board: the other
// task carrying the same title.
func findClone(t *testing.T, c *client, boardID, origID, title string) map[string]any {
	t.Helper()
	for _, m := range c.get("/boards/" + boardID + "/tasks").listBody(t) {
		if m["title"] != title || m["id"] == origID {
			continue
		}
		return c.expect(t, c.get("/tasks/"+m["id"].(string)), http.StatusOK)
	}
	t.Fatalf("no recurrence duplicate of %s on the board", origID)
	return nil
}
