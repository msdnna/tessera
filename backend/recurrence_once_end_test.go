// The end of a recurrence. The product expresses it two ways and only two: the
// «Повторять всегда» checkbox off (`once` — one more occurrence, then stop) and a
// custom date list running out. Neither had an end-to-end test; this is the probe
// that says what each one actually does to the card.
package main

import (
	"net/http"
	"testing"
	"time"
)

func rfc3339(ts time.Time) string { return ts.Format(time.RFC3339) }

// A one-off recurrence («Повторять всегда» unticked) hands over exactly one more
// occurrence and then leaves an ordinary task behind — on the reschedule branch
// and on the duplicate one alike.
func TestOnceRecurrenceStopsAfterOneOccurrence(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)
	due := time.Now().AddDate(0, 0, 3).UTC().Truncate(time.Second)

	// ── reschedule branch ───────────────────────────────────────────
	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Отвезти документы", "column_id": todo, "due_date": rfc3339(due),
	}), http.StatusCreated)
	id := task["id"].(string)
	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"due_date":   rfc3339(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1, "once": true},
	}), http.StatusOK)

	first := c.expect(t, c.patch("/tasks/"+id, map[string]any{"completed": true}), http.StatusOK)
	if first["completed_at"] != nil {
		t.Fatalf("the one-off did not hand over its single repeat — the card stayed closed: %v", first)
	}
	gotDue, err := time.Parse(time.RFC3339, first["due_date"].(string))
	if err != nil {
		t.Fatalf("due_date after the repeat: %v", err)
	}
	if want := due.AddDate(0, 0, 1); !gotDue.Equal(want) {
		t.Fatalf("the single repeat landed on %s, want %s", gotDue, want)
	}
	if first["recurrence"] != nil {
		t.Fatalf("the rule survived its one-off: recurrence=%v — the series repeats forever", first["recurrence"])
	}

	// Closing it again is an ordinary close: no third occurrence.
	second := c.expect(t, c.patch("/tasks/"+id, map[string]any{"completed": true}), http.StatusOK)
	if second["completed_at"] == nil {
		t.Fatalf("the task recurred a second time — «повторить один раз» did not stop it: %v", second)
	}
	if again, _ := time.Parse(time.RFC3339, second["due_date"].(string)); !again.Equal(gotDue) {
		t.Fatalf("the due date moved again after the one-off ended: %s → %s", gotDue, again)
	}

	// ── duplicate branch ────────────────────────────────────────────
	dup := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Сдать отчёт", "column_id": todo, "due_date": rfc3339(due),
	}), http.StatusCreated)
	dupID := dup["id"].(string)
	c.expect(t, c.patch("/tasks/"+dupID, map[string]any{
		"due_date": rfc3339(due),
		"recurrence": map[string]any{
			"freq": "daily", "interval": 1, "once": true, "create_new": true,
		},
	}), http.StatusOK)
	original := c.expect(t, c.patch("/tasks/"+dupID, map[string]any{"completed": true}), http.StatusOK)
	if original["recurrence"] != nil {
		t.Fatalf("the spent occurrence kept the rule: %v", original["recurrence"])
	}

	clone := findByTitle(t, c, s.Board, "Сдать отчёт", dupID)
	if clone == nil {
		t.Fatal("the duplicate branch created no next occurrence at all")
	}
	if clone["recurrence"] != nil {
		t.Fatalf("the duplicate carried the one-off rule forward: recurrence=%v — "+
			"«повторить один раз» repeats forever on this branch", clone["recurrence"])
	}
}

// A custom date list that has run out ends the series: the rule comes off the
// card and the due date stays where the last occurrence left it.
func TestCustomDatesExhaustedEndsTheSeries(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)

	d1 := time.Now().AddDate(0, 0, 2).UTC().Truncate(time.Second)
	d2 := d1.AddDate(0, 0, 5)
	// The picked days are UTC days — that is the zone they were formatted in when
	// they went into the rule — so the day read back has to be taken in UTC too.
	// The API answers in the server's zone (+03:00 here), and for any run between
	// 21:00 and midnight UTC that zone is already on the next day: without the
	// normalization the check fails every night on a perfectly good series.
	key := func(ts time.Time) string { return ts.UTC().Format("2006-01-02") }

	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Поход к врачу", "column_id": todo, "due_date": rfc3339(d1),
	}), http.StatusCreated)
	id := task["id"].(string)
	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"due_date": rfc3339(d1),
		"recurrence": map[string]any{
			"freq": "custom", "interval": 1, "dates": []string{key(d1), key(d2)},
		},
	}), http.StatusOK)

	first := c.expect(t, c.patch("/tasks/"+id, map[string]any{"completed": true}), http.StatusOK)
	got, err := time.Parse(time.RFC3339, first["due_date"].(string))
	if err != nil {
		t.Fatalf("due_date after the first custom occurrence: %v", err)
	}
	if key(got) != key(d2) {
		t.Fatalf("the second custom date did not arrive: due=%s, want the day %s", got, key(d2))
	}
	if first["recurrence"] == nil {
		t.Fatalf("the rule was dropped while a picked date was still ahead: %v", first)
	}

	last := c.expect(t, c.patch("/tasks/"+id, map[string]any{"completed": true}), http.StatusOK)
	if last["completed_at"] == nil {
		t.Fatalf("the series produced an occurrence past its last picked date: %v", last)
	}
	if last["recurrence"] != nil {
		t.Fatalf("an exhausted date list left the rule on the card: %v — the modal "+
			"keeps promising a repeat that is never coming", last["recurrence"])
	}
}

// findByTitle returns the board's task with that title whose id is not `skip`.
func findByTitle(t *testing.T, c *client, boardID, title, skip string) map[string]any {
	t.Helper()
	for _, m := range c.get("/boards/" + boardID + "/tasks").listBody(t) {
		if m["title"] == title && m["id"] != skip {
			return m
		}
	}
	return nil
}
