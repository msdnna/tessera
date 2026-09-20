package recur

import (
	"testing"
	"time"
)

// A weekly rule that names its own weekdays keeps them and keeps its interval,
// even when the skip-weekends flag arrives alongside them.
//
// The two together used to fight: applySkip pushed the chosen Saturday to Monday,
// and that pushed day became the anchor of the next step — one Mon-started week
// further on, where nextWeekly found the same Saturday five days out instead of a
// fortnight. From the second occurrence «every 2 weeks on Saturdays» was plain
// weekly Mondays, and the editor's chips went on showing Saturday (#2932).
func TestWeeklyWeekdaysKeepIntervalUnderSkipWeekends(t *testing.T) {
	rule, ok := Rule{Freq: FreqWeekly, Interval: 2, Weekdays: []int{6}, SkipWeekends: true}.normalized()
	if !ok {
		t.Fatal("rule did not normalize")
	}
	if rule.SkipWeekends {
		t.Fatal("skip_weekends survived an explicit weekday set")
	}
	cur := time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC) // Saturday
	for i := 0; i < 4; i++ {
		next, ok := rule.Next(cur)
		if !ok {
			t.Fatalf("occurrence %d: no next", i+1)
		}
		if next.Weekday() != time.Saturday {
			t.Errorf("occurrence %d: landed on %s, want Saturday", i+1, next.Weekday())
		}
		if gap := next.Sub(cur); gap != 14*24*time.Hour {
			t.Errorf("occurrence %d: gap %v, want 336h", i+1, gap)
		}
		cur = next
	}
}

// The flag still does its job where the day is derived rather than chosen: a
// weekly rule with no weekday set repeats on the due date's own weekday, and a
// weekend one is pushed off — that is the whole reason the flag exists.
func TestWeeklyWithoutWeekdaysStillSkipsWeekends(t *testing.T) {
	rule, ok := Rule{Freq: FreqWeekly, Interval: 1, SkipWeekends: true}.normalized()
	if !ok {
		t.Fatal("rule did not normalize")
	}
	if !rule.SkipWeekends {
		t.Fatal("skip_weekends dropped from a rule that has no weekday set")
	}
	next, ok := rule.Next(time.Date(2026, 3, 7, 9, 0, 0, 0, time.UTC)) // Saturday
	if !ok {
		t.Fatal("no next")
	}
	if next.Weekday() != time.Monday {
		t.Errorf("landed on %s, want Monday", next.Weekday())
	}
}

// Daily is untouched: there is no weekday set to contradict, so the push stays.
func TestDailyStillSkipsWeekends(t *testing.T) {
	rule, ok := Rule{Freq: FreqDaily, Interval: 1, SkipWeekends: true}.normalized()
	if !ok {
		t.Fatal("rule did not normalize")
	}
	next, ok := rule.Next(time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC)) // Friday
	if !ok {
		t.Fatal("no next")
	}
	if next.Weekday() != time.Monday {
		t.Errorf("landed on %s, want Monday", next.Weekday())
	}
}

// A weekday set that names no weekend day never met the push in the first place,
// so dropping the flag leaves that series exactly where it was.
func TestWeeklyWeekdayNoWeekendUnchanged(t *testing.T) {
	rule, ok := Rule{Freq: FreqWeekly, Interval: 2, Weekdays: []int{5}, SkipWeekends: true}.normalized()
	if !ok {
		t.Fatal("rule did not normalize")
	}
	cur := time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC) // Friday
	for i := 0; i < 3; i++ {
		next, ok := rule.Next(cur)
		if !ok {
			t.Fatalf("occurrence %d: no next", i+1)
		}
		if next.Weekday() != time.Friday || next.Sub(cur) != 14*24*time.Hour {
			t.Errorf("occurrence %d: %s +%v, want Friday +336h", i+1, next.Weekday(), next.Sub(cur))
		}
		cur = next
	}
}
