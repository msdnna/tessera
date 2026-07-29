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
		if oa, ob := isOverdue(a, now), isOverdue(b, now); oa != ob {
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

// isOverdue reports whether an incomplete task's due date is in the past.
func isOverdue(t model.Task, now time.Time) bool {
	return t.CompletedAt == nil && t.DueDate != nil && t.DueDate.Before(now)
}
