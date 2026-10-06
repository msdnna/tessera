// The clock-driven trigger («По расписанию») is the one nobody has to touch for
// the series to move: a background sweep advances every task whose due date has
// passed. Its only end-to-end test so far drives the branch that reschedules the
// same task after someone closed it — these cover the rest of what the sweep
// promises: the duplicate branch, occurrences missed while the server was down,
// and a rule that has run out.
package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// sweep runs one pass of the recurrence worker and returns once a pass has
// actually happened.
//
// AdvanceRecurrencesOnce answers false for one reason — somebody else holds the
// advisory lock — and somebody else always might: the harness runs the real
// workers, and the neighbouring tests in this file call this too. A sweep is
// short, so the lock is free again within moments; treating the first refusal as
// a verdict made these tests fail on a race with the very worker they exercise,
// not on the behaviour under test. Retrying until the budget runs out is the
// honest reading of "did the sweep run", and a foreign pass in between changes
// nothing: every assertion here is about the end state, which a second sweep
// cannot undo (a task it advanced has a future due date and drops out of the
// query).
func sweep(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if testAPI.AdvanceRecurrencesOnce(context.Background()) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the recurrence sweep never got the advisory lock in 10s")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// The sweep is global — it advances every overdue schedule task in the database,
// including those of tests running beside this one — so every assertion here is
// about the *end state*, which a second, foreign sweep cannot change: once a task
// is advanced its due date is in the future and the query stops selecting it.
func TestScheduleWorkerAdvancesWithoutAnyoneClosingTheTask(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)

	// «Скорее всего, сервер не работал какое-то время»: a due date well in the
	// past, so the sweep has more than one missed occurrence to reason about.
	past := time.Now().Add(-30*24*time.Hour - 12*time.Hour).UTC().Truncate(time.Second)
	dueOf := func(m map[string]any, what string) time.Time {
		t.Helper()
		raw, ok := m["due_date"].(string)
		if !ok {
			t.Fatalf("%s has no due date: %v", what, m["due_date"])
		}
		got, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t.Fatalf("%s: due date %v: %v", what, raw, err)
		}
		return got.UTC()
	}
	mk := func(t *testing.T, title string, rule map[string]any) string {
		t.Helper()
		task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": todo, "due_date": rfc3339(past),
		}), http.StatusCreated)
		id := task["id"].(string)
		// Rule and date in one write: the worker ticks every minute, and a task
		// that sat here overdue with a rule on it could be advanced mid-fixture.
		c.expect(t, c.patch("/tasks/"+id, map[string]any{
			"due_date": rfc3339(past), "recurrence": rule,
		}), http.StatusOK)
		return id
	}

	// A missed period is skipped, not replayed. NextAfter walks the calendar to
	// the first occurrence still ahead, so a month of downtime costs one advance
	// — replaying each missed day instead would bury the card's history under
	// thirty «recurred» lines and fire thirty notifications for deadlines that
	// are already gone.
	t.Run("пропущенные повторения не отыгрываются по одному", func(t *testing.T) {
		id := mk(t, "Бэкап базы", map[string]any{
			"freq": "daily", "interval": 1, "trigger": "schedule",
		})
		sweep(t)

		want := past
		for !want.After(time.Now().UTC()) {
			want = want.AddDate(0, 0, 1)
		}
		got := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
		if d := dueOf(got, "the advanced task"); !d.Equal(want) {
			t.Fatalf("due date %s, want the first occurrence still ahead (%s): "+
				"a month of downtime must cost one step, not thirty", d, want)
		}
		if n := countEvents(t, c, id, "recurred"); n != 1 {
			t.Fatalf("записей recurred = %d, want 1: the sweep replayed the missed occurrences", n)
		}
		if got["recurrence"] == nil {
			t.Fatal("the sweep dropped the rule — the series ends at its first tick")
		}
	})

	// The duplicate branch under the clock: nobody closed anything, so the sweep
	// is the only actor. The original becomes a spent occurrence (its rule
	// stripped, its date left alone) and the copy is born on the next date that
	// is actually ahead — the same NextAfter the reschedule branch gets.
	t.Run("копия рождается на будущем сроке", func(t *testing.T) {
		id := mk(t, "Еженедельная выгрузка", map[string]any{
			"freq": "weekly", "interval": 1, "trigger": "schedule", "create_new": true,
		})
		sweep(t)

		orig := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
		if orig["recurrence"] != nil {
			t.Fatal("the spent occurrence kept the rule — the sweep would copy it again every tick")
		}
		if d := dueOf(orig, "the spent occurrence"); !d.Equal(past) {
			t.Fatalf("the spent occurrence was re-dated: due %s, want the original %s", d, past)
		}

		clone := findClone(t, c, s.Board, id, "Еженедельная выгрузка")
		if d := dueOf(clone, "the duplicate"); !d.After(time.Now().UTC()) {
			t.Fatalf("the duplicate is born overdue (%s): the sweep would copy it again "+
				"on the next tick, and again, until the calendar catches up", d)
		}
		if clone["recurrence"] == nil {
			t.Fatal("the duplicate carries no rule — the series ends after one copy")
		}
		if clone["completed_at"] != nil {
			t.Fatalf("the duplicate was born closed: %v", clone["completed_at"])
		}
		if clone["column_id"] != todo {
			t.Fatalf("the duplicate landed in column %v, want the board's first one %v",
				clone["column_id"], todo)
		}
	})

	// A rule whose hand-picked dates are all behind us has no next occurrence.
	// The sweep has to take the rule off the card, or the query keeps handing the
	// task back every minute for an advance that can never happen.
	t.Run("исчерпанное правило снимается", func(t *testing.T) {
		d1 := past.AddDate(0, 0, -2).Format("2006-01-02")
		d2 := past.AddDate(0, 0, -1).Format("2006-01-02")
		id := mk(t, "Инвентаризация", map[string]any{
			"freq": "custom", "interval": 1, "trigger": "schedule",
			"dates": []string{d1, d2},
		})
		sweep(t)

		got := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
		if got["recurrence"] != nil {
			t.Fatalf("the exhausted rule is still on the card: %v", got["recurrence"])
		}
		if d := dueOf(got, "the exhausted task"); !d.Equal(past) {
			t.Fatalf("the task was advanced past the end of its rule: due %s, want %s", d, past)
		}
		if n := countEvents(t, c, id, "recurred"); n != 0 {
			t.Fatalf("записей recurred = %d, want 0: there was no occurrence to advance to", n)
		}
	})
}
