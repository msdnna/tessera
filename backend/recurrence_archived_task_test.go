// A recurrence must not advance a task that is sitting in the archive (#2932).
//
// The project states the invariant three times over, each time about a different
// piece of the same machinery: ListScheduleRecurDue filters archived_at ("the
// archive is the one place where a card is meant to stop moving, and a series
// resurrecting itself out of it would be a card nobody can see arriving"),
// cloneChecklist skips an archived item and everything under it, and
// carryChecklistFrom does the same in the reschedule branch. The fourth door was
// left open: the root task's own advance. recurAdvance looks at the rule, the
// trigger and the due date, and at nothing else — so the "complete" and "column"
// triggers walk straight past the archive that the "schedule" trigger respects.
//
// It is not a hypothetical route. Archiving is a board-wide act and a board is a
// shared, long-lived screen: one person archives the card, another still has the
// old board open and ticks its checkbox (or drags it into the trigger column),
// and the PATCH carries no archived guard. The API and the mobile client reach it
// the same way, and nothing in the handler refuses a write to an archived row.
//
// What comes out differs per branch, and both are the failure the other three
// guards name:
//   - create_new spawns a LIVE duplicate on the board, descended from a card that
//     is not on the board, tags, assignees, files and checklist included — and it
//     does it again on every occurrence.
//   - reschedule clears completed_at and steps the due date on the archived row
//     itself, so the item "went into the archive finished and came out open, with
//     a deadline in the future" — the exact sentence carryChecklistFrom's doc uses
//     about the archived checklist item it was fixed to skip.
package main

import (
	"net/http"
	"testing"
	"time"
)

// countLiveTitled counts a board's live (non-archived) tasks with a given title.
func countLiveTitled(t *testing.T, c *client, boardID, title string) int {
	t.Helper()
	n := 0
	for _, m := range c.get("/boards/" + boardID + "/tasks").listBody(t) {
		if m["title"] == title {
			n++
		}
	}
	return n
}

// TestRecurrenceDoesNotAdvanceArchivedTask: the create_new branch — closing an
// archived recurring card must not put a fresh occurrence on the board.
func TestRecurrenceDoesNotAdvanceArchivedTask(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo := s.col(t, 0)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Еженедельный отчёт", "column_id": todo, "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	// A checklist item, so a resurrected series is visible as a whole plan and not
	// just one stray title.
	c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Свести цифры", "column_id": todo, "parent_id": origID,
	}), http.StatusCreated)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "weekly", "interval": 1, "create_new": true},
	}), http.StatusOK)

	// The card leaves the board. The routine is over — that is what archiving it
	// means.
	c.expect(t, c.patch("/tasks/"+origID+"/archive", nil), http.StatusNoContent)

	// Somebody's board was a minute stale: the checkbox goes in on a card that is
	// no longer there.
	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Еженедельный отчёт", "due_date": rfc(due), "completed": true,
	}), http.StatusOK)

	if got := countLiveTitled(t, c, s.Board, "Еженедельный отчёт"); got != 0 {
		t.Fatalf("the archive spawned %d live occurrences of the card, want 0", got)
	}
	if got := countLiveTitled(t, c, s.Board, "Свести цифры"); got != 0 {
		t.Fatalf("the archive spawned %d live checklist items, want 0", got)
	}
	// Exactly one entry in the archive: not restored, and not duplicated into it.
	if got := countArchivedTitled(t, c, s.Board, "Еженедельный отчёт"); got != 1 {
		t.Fatalf("archive holds %d copies of the card, want 1", got)
	}

	// The series is not lost either — it is suspended. An advance that did happen
	// would have stripped the rule off this row ("it's now a historical instance"),
	// so restoring the card would give back a plain task with no repeat on it.
	c.expect(t, c.patch("/tasks/"+origID+"/restore", nil), http.StatusNoContent)
	back := c.expect(t, c.get("/tasks/"+origID), http.StatusOK)
	if back["recurrence"] == nil {
		t.Fatalf("the archived card came back without its repeat rule: %v", back)
	}
}

// TestRescheduleDoesNotReopenArchivedTask: the reschedule branch — the archived
// row itself must not be re-dated and re-opened.
func TestRescheduleDoesNotReopenArchivedTask(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }

	task := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	id := task["id"].(string)

	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
	}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+id+"/archive", nil), http.StatusNoContent)

	c.expect(t, c.patch("/tasks/"+id, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due), "completed": true,
	}), http.StatusOK)

	if got := countLiveTitled(t, c, s.Board, "Ежедневный статус"); got != 0 {
		t.Fatalf("the reschedule put the archived card back on the board (%d live copies)", got)
	}

	// What the archive gives back has to be what went in: closed, and dated for
	// the occurrence it was closed on.
	c.expect(t, c.patch("/tasks/"+id+"/restore", nil), http.StatusNoContent)
	back := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
	if back["completed_at"] == nil {
		t.Fatalf("a card archived closed came back open: %v", back["completed_at"])
	}
	if got := parseTS(t, back["due_date"]); !got.Equal(due) {
		t.Fatalf("the archived card's due date stepped to %s, want %s", got, due)
	}
}
