package recur

import (
	"testing"
	"time"
)

// A due date picked in the editor defaults to *local* midnight, so for anyone east
// of UTC the instant that reaches the server belongs to the previous calendar day.
// Every one of these rules is calendar arithmetic — weekday, day-of-month, "is this
// a weekend", "is this the date I clicked" — so doing it in the server's zone shifts
// the whole series by a day while the editor's calendar keeps highlighting the days
// the user actually chose.
//
// Europe/Moscow is +03:00 year-round (no DST since 2014), which keeps these cases
// about the zone and not about a transition.
func mskMidnight(t *testing.T, y int, m time.Month, d int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("no tzdata for Europe/Moscow: %v", err)
	}
	return time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
}

// mskDay renders an instant as the calendar day the user sees.
func mskDay(t *testing.T, at time.Time) string {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("no tzdata for Europe/Moscow: %v", err)
	}
	return at.In(loc).Format("2006-01-02 Mon 15:04")
}

func TestNextHonoursRuleTimezone(t *testing.T) {
	const msk = "Europe/Moscow"
	cases := []struct {
		name string
		rule Rule
		from time.Time
		want string
	}{
		{
			// "every Monday", due Monday 2026-04-06 00:00 MSK (= Sunday 21:00 UTC).
			name: "weekly weekday stays on the chosen weekday",
			rule: Rule{Freq: FreqWeekly, Interval: 1, Weekdays: []int{1}, TZ: msk},
			from: mskMidnight(t, 2026, time.April, 6),
			want: "2026-04-13 Mon 00:00",
		},
		{
			// Daily standup at 00:00 that must not land on a weekend: Friday's next
			// occurrence is Monday, not Saturday.
			name: "daily skip_weekends skips the user's weekend",
			rule: Rule{Freq: FreqDaily, Interval: 1, SkipWeekends: true, TZ: msk},
			from: mskMidnight(t, 2026, time.April, 10), // Friday
			want: "2026-04-13 Mon 00:00",
		},
		{
			// Hand-picked calendar dates have to fire on the day that was clicked.
			name: "custom date fires on the day it was picked",
			rule: Rule{Freq: FreqCustom, Interval: 1, Dates: []string{"2026-04-10", "2026-04-20"}, TZ: msk},
			from: mskMidnight(t, 2026, time.April, 10),
			want: "2026-04-20 Mon 00:00",
		},
		{
			// Monthly on the 1st: the anchor is the day the user sees, not the UTC one.
			name: "monthly keeps the user's day of month",
			rule: Rule{Freq: FreqMonthly, Interval: 1, Day: 1, TZ: msk},
			from: mskMidnight(t, 2026, time.April, 1),
			want: "2026-05-01 Fri 00:00",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rule, ok := c.rule.normalized()
			if !ok {
				t.Fatalf("rule did not normalize")
			}
			got, ok := rule.Next(c.from)
			if !ok {
				t.Fatalf("Next ok=false")
			}
			if day := mskDay(t, got); day != c.want {
				t.Fatalf("Next = %s, want %s", day, c.want)
			}
		})
	}
}

// The anchor is what keeps a monthly rule on the 31st after February clamps it, so
// it has to be read in the rule's zone too — otherwise a due date saved as local
// midnight anchors the series to the previous day for good.
func TestWithAnchorUsesRuleTimezone(t *testing.T) {
	const msk = "Europe/Moscow"
	due := mskMidnight(t, 2026, time.April, 1) // 2026-03-31T21:00Z
	got := Rule{Freq: FreqMonthly, Interval: 1, TZ: msk}.WithAnchor(due)
	if got.Day != 1 {
		t.Fatalf("anchor day = %d, want 1 (the day the user picked)", got.Day)
	}
}

// An unknown or blank zone must not make the rule unusable — it falls back to UTC,
// which is exactly what every rule stored before this field did.
func TestTimezoneFallbacks(t *testing.T) {
	for _, tz := range []string{"", "Mars/Olympus", "  "} {
		r, ok := Rule{Freq: FreqDaily, Interval: 1, TZ: tz}.normalized()
		if !ok {
			t.Fatalf("tz %q made the rule invalid", tz)
		}
		if r.TZ != "" {
			t.Fatalf("tz %q survived normalization as %q", tz, r.TZ)
		}
		from := time.Date(2026, time.April, 10, 21, 0, 0, 0, time.UTC)
		next, _ := r.Next(from)
		if want := from.AddDate(0, 0, 1); !next.Equal(want) {
			t.Fatalf("Next = %v, want %v (UTC arithmetic)", next, want)
		}
	}
}
