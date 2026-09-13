package handlers

import (
	"testing"
	"time"
)

// TestTaskDateStr pins how a task's own date field becomes the calendar day
// GitLab carries.
//
// A task due date is an instant, not a date-only value: the editors build it
// from local calendar fields (DueEditor.vue picks `new Date(y, m, d, 0, 0, 0, 0)`)
// and «/due сегодня» resolves to midnight in the caller's zone. Read in UTC,
// every such date east of UTC names the previous day — the day the GitLab issue
// then shows, and the day the conflict detector compares against the snapshot.
//
// The one case that must stay UTC is a genuine date-only value (pure UTC
// midnight — what a pull from GitLab stores), which the clients already read in
// UTC for exactly this reason (isDateOnly in frontend/src/utils/format.js).
func TestTaskDateStr(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("load Europe/Moscow: %v", err)
	}
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load America/New_York: %v", err)
	}
	at := func(loc *time.Location, y int, m time.Month, d, hh, mm int) *time.Time {
		v := time.Date(y, m, d, hh, mm, 0, 0, loc)
		return &v
	}
	cases := []struct {
		name string
		due  *time.Time
		loc  *time.Location
		want string
	}{
		{
			// 2026-08-14 00:00 +03:00 == 2026-08-13T21:00Z. The author picked the 14th.
			name: "local midnight east of UTC keeps its day",
			due:  at(msk, 2026, time.August, 14, 0, 0),
			loc:  msk,
			want: "2026-08-14",
		},
		{
			// 2026-08-14 21:00 -04:00 == 2026-08-15T01:00Z. The author picked the 14th.
			name: "evening west of UTC keeps its day",
			due:  at(ny, 2026, time.August, 14, 21, 0),
			loc:  ny,
			want: "2026-08-14",
		},
		{
			// What a pull from GitLab stores: a date-only value, the same day everywhere.
			name: "pure UTC midnight stays date-only",
			due:  at(time.UTC, 2026, time.August, 14, 0, 0),
			loc:  ny,
			want: "2026-08-14",
		},
		{
			name: "no due date is the empty string",
			due:  nil,
			loc:  msk,
			want: "",
		},
		{
			// A nil zone must not panic the writeback worker: it means "unknown", and
			// UTC is the behaviour rules stored before this existed already had.
			name: "unknown zone falls back to UTC",
			due:  at(time.UTC, 2026, time.August, 14, 12, 0),
			loc:  nil,
			want: "2026-08-14",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := taskDateStr(tc.due, tc.loc); got != tc.want {
				t.Fatalf("taskDateStr = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTaskDateStrConflictOurs is the detector's side of the same mistake: a due
// date that came from GitLab and was re-saved from the editor (which hands back
// local midnight of the same day) must still read as the day GitLab has, or the
// three-way check parks a conflict on a date nobody moved.
func TestTaskDateStrConflictOurs(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("load Europe/Moscow: %v", err)
	}
	// GitLab says the 14th; the task holds the 14th as Moscow local midnight.
	theirs := time.Date(2026, time.August, 14, 0, 0, 0, 0, time.UTC)
	ours := time.Date(2026, time.August, 14, 0, 0, 0, 0, msk)
	if got, want := taskDateStr(&ours, msk), dateStr(&theirs); got != want {
		t.Fatalf("ours = %q, theirs = %q — same day must compare equal", got, want)
	}
}
