package handlers

import (
	"testing"
	"time"

	"tessera/internal/db"
)

func TestDueShouldFire(t *testing.T) {
	due := time.Date(2026, 6, 16, 18, 0, 0, 0, time.UTC)
	state := func(firedDue time.Time, last time.Time) *db.DueNotificationState {
		return &db.DueNotificationState{FiredDue: firedDue, LastFiredAt: last}
	}

	cases := []struct {
		name   string
		now    time.Time
		lead   int32
		repeat int32
		prior  *db.DueNotificationState
		want   bool
	}{
		{"before lead window", due.Add(-90 * time.Minute), 60, 0, nil, false},
		{"inside lead window, never fired", due.Add(-30 * time.Minute), 60, 0, nil, true},
		{"one-shot already fired", due.Add(-30 * time.Minute), 60, 0, state(due, due.Add(-31*time.Minute)), false},
		{"due date moved → re-arm", due.Add(-30 * time.Minute), 60, 0, state(due.Add(-time.Hour), due.Add(-90*time.Minute)), true},
		{"repeat not yet elapsed", due.Add(-30 * time.Minute), 60, 15, state(due, due.Add(-25*time.Minute)), false},
		{"repeat elapsed", due.Add(-5 * time.Minute), 60, 15, state(due, due.Add(-25*time.Minute)), true},
		{"overdue still repeats", due.Add(40 * time.Minute), 60, 15, state(due, due.Add(20*time.Minute)), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dueShouldFire(tc.now, due, tc.lead, tc.repeat, tc.prior); got != tc.want {
				t.Fatalf("dueShouldFire = %v, want %v", got, tc.want)
			}
		})
	}
}
