// A recurrence must carry a task's own due-notification settings into the next
// occurrence.
//
// The lead, the repeat and the on/off switch live as columns on `tasks` (each NULL
// = inherit the user's default), and the card's due popover is where a person sets
// them: "this daily chore is noisy, don't warn me about it" or "warn me three days
// ahead about the monthly report, an hour is useless". The two recurrence branches
// disagreed about them. The reschedule branch edits the task in place and UpdateTask
// never touches those columns, so the settings survive; the create_new branch builds
// the next occurrence with CreateTask, which is born with all three NULL. So the
// muted chore started notifying again the very next day, and the report that was to
// warn three days ahead warned an hour ahead — silently, with the popover of the
// spent occurrence still showing what the user had asked for.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestRecurrenceCarriesDueNotifyOverrides(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	// ── the create_new branch: the original stays, a duplicate carries on ──
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Месячный отчёт", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	// A checklist item of its own: the duplicate rebuilds the whole checklist, so
	// an item's settings go the same way the parent's do.
	step := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свести цифры", "column_id": s.col(t, 0), "parent_id": origID,
	}), http.StatusCreated)

	// What the user asked for on the card: warn three days ahead, then every six
	// hours. On the item: don't warn at all.
	c.expect(t, c.patch("/tasks/"+origID+"/due-notify", map[string]any{
		"lead_minutes": 4320, "repeat_minutes": 360, "enabled": true,
	}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+step["id"].(string)+"/due-notify", map[string]any{
		"enabled": false,
	}), http.StatusOK)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Месячный отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "monthly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Месячный отчёт")
	if got := clone["due_lead_minutes"]; got != float64(4320) {
		t.Fatalf("the duplicate warns with lead %v, want the 4320 the user set (an hour by default)", got)
	}
	if got := clone["due_repeat_minutes"]; got != float64(360) {
		t.Fatalf("the duplicate repeats every %v minutes, want the 360 the user set", got)
	}
	if got := clone["due_notify_enabled"]; got != true {
		t.Fatalf("the duplicate lost the explicit due-notify switch: %v", got)
	}
	cloneStep := subtaskByTitle(t, clone, "Свести цифры")
	stepDetail := c.expect(t, c.get("/tasks/"+cloneStep["id"].(string)), http.StatusOK)
	if got := stepDetail["due_notify_enabled"]; got != false {
		t.Fatalf("the duplicate's checklist item is notifying again after being muted: %v", got)
	}

	// The spent occurrence keeps its own settings — the duplicate copied them, it
	// did not take them.
	first := c.expect(t, c.get("/tasks/"+origID), http.StatusOK)
	if got := first["due_lead_minutes"]; got != float64(4320) {
		t.Fatalf("the first occurrence lost its lead to the duplicate: %v", got)
	}

	// ── the reschedule branch: the same task moves to the next occurrence ──
	// It never rebuilt anything, so it is the control: the settings a person set
	// survive a repeat, which is what the duplicate has to match.
	daily := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	dailyID := daily["id"].(string)
	c.expect(t, c.patch("/tasks/"+dailyID+"/due-notify", map[string]any{
		"enabled": false,
	}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+dailyID, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)
	next := c.expect(t, c.get("/tasks/"+dailyID), http.StatusOK)
	if got := next["due_notify_enabled"]; got != false {
		t.Fatalf("a reschedule unmuted the task: %v", got)
	}

	// A task nobody customised stays uncustomised: the carry copies what was set,
	// it does not pin the defaults into the row.
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
	if got := plainClone["due_lead_minutes"]; got != nil {
		t.Fatalf("the duplicate froze the user default into its own row: lead=%v", got)
	}
	if got := plainClone["due_notify_enabled"]; got != nil {
		t.Fatalf("the duplicate froze the user default into its own row: enabled=%v", got)
	}
}
