package confsched

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func TestNextOneOffStaysPut(t *testing.T) {
	start := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	if got := Next(start, None, time.UTC, 30*time.Minute, now); !got.Equal(start) {
		t.Fatalf("one-off moved: %v", got)
	}
}

func TestNextFutureStartIsItself(t *testing.T) {
	start := time.Date(2026, 10, 9, 9, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	if got := Next(start, Daily, time.UTC, 0, now); !got.Equal(start) {
		t.Fatalf("future start moved: %v", got)
	}
}

func TestNextDailyKeepsMeetingInProgress(t *testing.T) {
	start := time.Date(2026, 9, 1, 17, 30, 0, 0, time.UTC)
	// 17:40 — the 17:30 standup of 15 minutes is still on.
	now := time.Date(2026, 10, 8, 17, 40, 0, 0, time.UTC)
	want := time.Date(2026, 10, 8, 17, 30, 0, 0, time.UTC)
	if got := Next(start, Daily, time.UTC, 15*time.Minute, now); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// 17:46 — over, tomorrow's.
	now = time.Date(2026, 10, 8, 17, 46, 0, 0, time.UTC)
	want = time.Date(2026, 10, 9, 17, 30, 0, 0, time.UTC)
	if got := Next(start, Daily, time.UTC, 15*time.Minute, now); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextWeekdaysSkipsWeekend(t *testing.T) {
	// Friday 2026-10-09 11:00 is over at 12:00 → Monday 12th.
	start := time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC) // Monday
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	want := time.Date(2026, 10, 12, 11, 0, 0, 0, time.UTC)
	if got := Next(start, Weekdays, time.UTC, 30*time.Minute, now); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// Saturday → Monday too.
	now = time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
	if got := Next(start, Weekdays, time.UTC, 30*time.Minute, now); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextBiweekly(t *testing.T) {
	start := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC) // Wed
	now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)
	// 2 Sep, 16 Sep, 30 Sep, 14 Oct.
	want := time.Date(2026, 10, 14, 11, 0, 0, 0, time.UTC)
	if got := Next(start, Biweekly, time.UTC, 45*time.Minute, now); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextWeeklyFarPast(t *testing.T) {
	start := time.Date(2020, 1, 6, 10, 0, 0, 0, time.UTC) // Monday
	now := time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)   // Thursday
	want := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	if got := Next(start, Weekly, time.UTC, 0, now); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextMonthlyClampsToMonthEnd(t *testing.T) {
	start := time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC)
	now := time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)
	want := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	if got := Next(start, Monthly, time.UTC, 0, now); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextKeepsWallClockAcrossDST(t *testing.T) {
	berlin := mustLoc(t, "Europe/Berlin")
	// 11:00 Berlin in summer (UTC+2) → after 25 Oct 2026 it is UTC+1.
	start := time.Date(2026, 10, 20, 11, 0, 0, 0, berlin)
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	got := Next(start, Daily, berlin, 0, now)
	if h := got.In(berlin).Hour(); h != 11 {
		t.Fatalf("wall clock drifted: %v", got.In(berlin))
	}
	if got.UTC().Hour() != 10 {
		t.Fatalf("expected 10:00 UTC after the switch, got %v", got.UTC())
	}
}

func TestValidAndLocation(t *testing.T) {
	for _, r := range []string{"", "daily", "weekdays", "weekly", "biweekly", "monthly"} {
		if !Valid(r) {
			t.Fatalf("%q should be valid", r)
		}
	}
	if Valid("yearly") {
		t.Fatal("yearly is not a preset")
	}
	if Location("") != time.UTC || Location("Nowhere/Land") != time.UTC {
		t.Fatal("bad zone must fall back to UTC")
	}
}
