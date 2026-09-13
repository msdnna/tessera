// A recurrence rule's zone has to survive an edit made by a client that does not
// know the field exists. The zone is what decides which calendar day the series
// lands on (internal/recur, Rule.TZ), so losing it silently moves every future
// occurrence — the "Mondays came out as Tuesdays" failure that field was added to
// fix, re-entered through a different door.
package main

import (
	"net/http"
	"testing"
	"time"
)

// A PATCH whose recurrence omits `tz` must keep the stored zone rather than
// reset the rule to UTC.
//
// The Android client's Recurrence model has no tz field at all, and its
// updateTask re-sends the whole rule by default — so renaming a task, changing
// its priority or ticking it off from the phone round-trips a web-authored rule
// back without the zone. Rules written before the field existed are the same
// shape. The anchor (day-of-month) is already carried over from the previous
// rule for exactly this reason; the zone is the same kind of server-held
// calendar state and needs the same treatment.
func TestRecurrenceKeepsZoneWhenClientOmitsIt(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("load Europe/Moscow: %v", err)
	}
	// Monday, local midnight — which is Sunday 21:00 in UTC. That gap is the
	// whole bug: stepped in UTC the series lands a day late.
	due := time.Date(2026, 3, 2, 0, 0, 0, 0, msk)

	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Понедельничная летучка", "column_id": s.col(t, 0),
		"due_date": due.Format(time.RFC3339),
	}), http.StatusCreated)
	id := task["id"].(string)

	// The web editor writes the rule with its zone.
	withTZ := c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Понедельничная летучка", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{
			"freq": "weekly", "interval": 1, "weekdays": []int{1}, "tz": "Europe/Moscow",
		},
	}), http.StatusOK)
	if tz := ruleField(t, withTZ, "tz"); tz != "Europe/Moscow" {
		t.Fatalf("stored rule tz = %v, want Europe/Moscow", tz)
	}

	// An unrelated edit from a client that knows nothing of tz: same rule, minus
	// the zone. The title is all that changed.
	renamed := c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Понедельничная летучка (еженедельно)", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "weekdays": []int{1}},
	}), http.StatusOK)
	if tz := ruleField(t, renamed, "tz"); tz != "Europe/Moscow" {
		t.Fatalf("rename dropped the rule's zone: tz = %v, want Europe/Moscow", tz)
	}

	// And the advance still lands on a Monday, not the Tuesday UTC arithmetic
	// produces for anyone east of it.
	adv := c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Понедельничная летучка (еженедельно)", "due_date": due.Format(time.RFC3339),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "weekdays": []int{1}},
		"completed":  true,
	}), http.StatusOK)
	next := parseTS(t, adv["due_date"]).In(msk)
	if next.Weekday() != time.Monday {
		t.Fatalf("next occurrence = %v (%v), want a Monday", next, next.Weekday())
	}
	if want := due.AddDate(0, 0, 7); !next.Equal(want) {
		t.Fatalf("next occurrence = %v, want %v", next, want)
	}
}

// ruleField reads one field of a task's stored recurrence rule.
func ruleField(t *testing.T, task map[string]any, key string) any {
	t.Helper()
	rule, ok := task["recurrence"].(map[string]any)
	if !ok {
		t.Fatalf("task has no recurrence rule: %v", task["recurrence"])
	}
	return rule[key]
}
