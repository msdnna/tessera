// A recurrence must leave an archived checklist item exactly as the archive took
// it — ticked or not, in the column it was closed in, on the dates it carried.
//
// Archiving one item of a repeating task says "this step is no longer part of the
// routine" while keeping the rest; it is a gesture with its own undo, and the item
// stands in the board's archive as an entry of its own while its parent stays
// active. The create_new branch already honours that (cloneChecklist skips an
// archived row, and skips what hangs below it). The reschedule branch carries the
// checklist with three statements that name parent_id and nothing else —
// ReopenSubtasks, LiftSubtasksFromColumn, ShiftSubtaskWindow — so they reached the
// archived rows too: every occurrence un-ticked the archived item, took it out of
// «Готово» into the rule's target column and pushed its dates one period further.
// archived_at itself is untouched, so the board never showed it and the older test
// («did not come back onto the board», «one copy in the archive») stayed green.
// The damage surfaces on restore: an item archived as finished, out of «Готово»,
// comes back a week later as an open card in «К работе» with a deadline a week in
// the future — and the dates keep creeping for as long as the routine repeats.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestRecurrenceLeavesArchivedChecklistItemAlone(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	todo, done := s.col(t, 0), s.col(t, 3)
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	parent := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный обход", "column_id": todo,
		"start_date": rfc(start), "due_date": rfc(due),
	}), http.StatusCreated)
	parentID := parent["id"].(string)

	mkItem := func(title string) string {
		t.Helper()
		item := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": todo, "parent_id": parentID,
			"start_date": rfc(start), "due_date": rfc(due),
		}), http.StatusCreated)
		id := item["id"].(string)
		closed := c.expect(t, c.patch("/tasks/"+id+"/move",
			map[string]any{"column_id": done}), http.StatusOK)
		if closed["completed_at"] == nil {
			t.Fatalf("fixture: the done column did not complete %q: %v", title, closed)
		}
		return id
	}

	// The control: an ordinary item of the routine, closed the board's way. It is
	// what the recurrence is supposed to carry — green before this fix and after.
	liveID := mkItem("Проверить бэкапы")
	// The item that left the routine: finished, then archived.
	goneID := mkItem("Снять слепок вручную")
	c.expect(t, c.patch("/tasks/"+goneID+"/archive", nil), http.StatusNoContent)

	c.expect(t, c.patch("/tasks/"+parentID, map[string]any{
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	advanced := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
	if got, want := parseTS(t, advanced["due_date"]), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("fixture: parent due = %v, want %v (the recurrence did not advance)", got, want)
	}

	// ── the control: the live item did travel to the next occurrence ──
	live := c.expect(t, c.get("/tasks/"+liveID), http.StatusOK)
	if live["completed_at"] != nil {
		t.Fatalf("the live checklist item stayed ticked into the next occurrence: %v", live)
	}
	if live["column_id"] != todo {
		t.Fatalf("the live checklist item stayed in «Готово»: column = %v", live["column_id"])
	}
	if got, want := parseTS(t, live["due_date"]), due.AddDate(0, 0, 1); !got.Equal(want) {
		t.Fatalf("the live checklist item kept the previous deadline: %v, want %v", got, want)
	}

	// ── the finding: the archived item must be exactly as it was archived ──
	gone := c.expect(t, c.get("/tasks/"+goneID), http.StatusOK)
	if gone["archived_at"] == nil {
		t.Fatalf("fixture: the item left the archive on its own: %v", gone)
	}
	if gone["completed_at"] == nil {
		t.Fatalf("the recurrence un-ticked an archived checklist item: %v", gone)
	}
	if gone["column_id"] != done {
		t.Fatalf("the recurrence moved an archived checklist item out of «Готово»: column = %v, want %v",
			gone["column_id"], done)
	}
	if got := parseTS(t, gone["due_date"]); !got.Equal(due) {
		t.Fatalf("the recurrence shifted an archived checklist item's deadline: %v, want %v", got, due)
	}
	if got := parseTS(t, gone["start_date"]); !got.Equal(start) {
		t.Fatalf("the recurrence shifted an archived checklist item's start: %v, want %v", got, start)
	}

	// ── and the way a person actually meets it: the undo ──
	c.expect(t, c.patch("/tasks/"+goneID+"/restore", nil), http.StatusNoContent)
	restored := c.expect(t, c.get("/tasks/"+goneID), http.StatusOK)
	if restored["completed_at"] == nil || restored["column_id"] != done {
		t.Fatalf("the archive gave back a different item than it took: completed=%v column=%v (want done, %v)",
			restored["completed_at"], restored["column_id"], done)
	}
	if got := parseTS(t, restored["due_date"]); !got.Equal(due) {
		t.Fatalf("the restored item came back with a deadline the recurrence invented: %v, want %v", got, due)
	}
}
