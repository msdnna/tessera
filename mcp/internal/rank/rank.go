// Package rank orders Tessera tasks into an actionable work queue: what to do
// next, by priority and deadline. Ranking lives client-side so the backend
// stays untouched (loose coupling).
package rank

import (
	"sort"
	"time"

	"tessera-mcp/internal/model"
)

// Options tune filtering and the "overdue" cutoff.
type Options struct {
	// IncludeCompleted keeps completed tasks in the result (ranked last).
	IncludeCompleted bool
	// Now is the reference time for the overdue check. Zero → time.Now().
	Now time.Time
}

// Tasks filters out archived (and, unless requested, completed) tasks and
// returns the rest in priority order. Ordering, by descending weight:
//
//  1. incomplete before completed
//  2. overdue before not-overdue
//  3. higher priority first (4=urgent … 0=none)
//  4. earlier due date first (no due date sorts last)
//  5. lower board position first (stable tie-break)
func Tasks(tasks []model.Task, opts Options) []model.Task {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	out := make([]model.Task, 0, len(tasks))
	for _, t := range tasks {
		if t.ArchivedAt != nil {
			continue
		}
		if t.CompletedAt != nil && !opts.IncludeCompleted {
			continue
		}
		out = append(out, t)
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]

		if ca, cb := a.CompletedAt != nil, b.CompletedAt != nil; ca != cb {
			return !ca // incomplete first
		}
		if oa, ob := IsOverdue(a, now), IsOverdue(b, now); oa != ob {
			return oa // overdue first
		}
		if a.Priority != b.Priority {
			return a.Priority > b.Priority // higher priority first
		}
		if da, db := a.DueDate, b.DueDate; (da != nil) != (db != nil) {
			return da != nil // tasks with a due date first
		} else if da != nil && db != nil && !da.Equal(*db) {
			return da.Before(*db) // earlier due first
		}
		return a.Position < b.Position
	})
	return out
}

// IsOverdue reports whether an incomplete task's due date has already passed.
//
// "Passed" means a whole day, not an instant, and that is the whole point of
// this function. A due date is stored as midnight in the writer's own calendar
// (resolvedTimezone in frontend/src/utils/format.js) and comes back over the API
// carrying that offset — `"due":"2026-09-18T00:00:00+03:00"` — so a task due
// *today* sits at an instant that is already behind `now` by one minute past
// midnight. Comparing the two directly therefore files every task due today as
// late, for the whole of the day it is actually due on.
//
// The rest of the product counts by day and this mirrors it: the backend asks
// for `due_date < @day_start` against the caller's local midnight
// (db/queries/workspace_tasks.sql) and the web for `dueDay < today`
// (frontend/src/utils/taskFilter.js). The backend already had this exact bug
// once — see commandNow in handlers/quick_actions.go, "«/due сегодня» used to
// file a task as already overdue".
//
// The day is the one `now` is in, in `now`'s own location: the caller passes the
// clock of whoever is reading the queue, which is the same calendar the due
// dates were written in.
func IsOverdue(t model.Task, now time.Time) bool {
	return t.CompletedAt == nil && t.DueDate != nil && t.DueDate.Before(startOfDay(now))
}

// startOfDay is midnight opening the day `now` falls in, in `now`'s location.
func startOfDay(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
}
