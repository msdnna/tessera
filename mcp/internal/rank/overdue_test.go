package rank

import (
	"testing"
	"time"

	"tessera-mcp/internal/model"
)

// msk is the zone a due date actually arrives in: the clients store a due date as
// midnight in the reader's own calendar (resolvedTimezone in
// frontend/src/utils/format.js), and the API hands it back with that offset —
// `"due":"2026-09-18T00:00:00+03:00"`.
var msk = time.FixedZone("MSK", 3*60*60)

func midnight(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, msk)
}

// TestOverdueIsCountedByDay pins the product's definition of "overdue", which is
// a whole day and not an instant: the backend counts `due_date < @day_start`
// against the caller's local midnight (db/queries/workspace_tasks.sql) and the
// web counts `dueDay < today` (frontend/src/utils/taskFilter.js). Reading it as
// an instant marks every task due *today* late from one minute past midnight.
func TestOverdueIsCountedByDay(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 15, 0, 0, msk)

	cases := []struct {
		name string
		task model.Task
		want bool
	}{
		{"due today", model.Task{DueDate: ptime(midnight(2026, 9, 14))}, false},
		{"due yesterday", model.Task{DueDate: ptime(midnight(2026, 9, 13))}, true},
		{"due tomorrow", model.Task{DueDate: ptime(midnight(2026, 9, 15))}, false},
		{"later today", model.Task{DueDate: ptime(time.Date(2026, 9, 14, 23, 59, 0, 0, msk))}, false},
		{
			"yesterday but completed",
			model.Task{DueDate: ptime(midnight(2026, 9, 13)), CompletedAt: ptime(now)},
			false,
		},
		{"no due date", model.Task{}, false},
	}
	for _, tc := range cases {
		if got := IsOverdue(tc.task, now); got != tc.want {
			t.Errorf("%s: IsOverdue = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestDueTodayDoesNotJumpTheQueue is the consequence the caller actually sees.
// The overdue bucket outranks priority, so reading "due today" as late pushes a
// routine task past an urgent one — and next_task hands the agent the wrong task.
func TestDueTodayDoesNotJumpTheQueue(t *testing.T) {
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, msk)
	dueToday := model.Task{ID: "due-today", Priority: 1, DueDate: ptime(midnight(2026, 9, 14))}
	urgent := model.Task{ID: "urgent", Priority: 4}
	reallyLate := model.Task{ID: "really-late", Priority: 0, DueDate: ptime(midnight(2026, 9, 12))}

	got := Tasks([]model.Task{dueToday, urgent, reallyLate}, Options{Now: now})

	want := []string{"really-late", "urgent", "due-today"}
	if len(got) != len(want) {
		t.Fatalf("got %d tasks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("position %d: got %q, want %q", i, got[i].ID, want[i])
		}
	}
}
