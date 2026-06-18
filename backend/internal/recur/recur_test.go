package recur

import (
	"encoding/json"
	"testing"
	"time"
)

func raw(s string) *json.RawMessage { r := json.RawMessage(s); return &r }

func TestParse(t *testing.T) {
	cases := []struct {
		name   string
		in     *json.RawMessage
		wantOK bool
		wantR  Rule
	}{
		{"nil", nil, false, Rule{}},
		{"empty", raw(``), false, Rule{}},
		{"garbage", raw(`not json`), false, Rule{}},
		{"unknown freq", raw(`{"freq":"hourly","interval":1}`), false, Rule{}},
		{"daily", raw(`{"freq":"daily","interval":3}`), true, Rule{Freq: FreqDaily, Interval: 3}},
		{"interval clamp", raw(`{"freq":"weekly","interval":0}`), true, Rule{Freq: FreqWeekly, Interval: 1}},
		{"negative interval", raw(`{"freq":"monthly","interval":-5}`), true, Rule{Freq: FreqMonthly, Interval: 1}},
		{"monthly with anchor", raw(`{"freq":"monthly","interval":1,"day":30}`), true, Rule{Freq: FreqMonthly, Interval: 1, Day: 30}},
		{"daily drops anchor", raw(`{"freq":"daily","interval":1,"day":30}`), true, Rule{Freq: FreqDaily, Interval: 1}},
		{"out-of-range day dropped", raw(`{"freq":"monthly","interval":1,"day":99}`), true, Rule{Freq: FreqMonthly, Interval: 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Parse(c.in)
			if ok != c.wantOK || got != c.wantR {
				t.Fatalf("Parse=%+v,%v want %+v,%v", got, ok, c.wantR, c.wantOK)
			}
		})
	}
}

func TestNext(t *testing.T) {
	d := func(y int, m time.Month, day, h, mins int) time.Time {
		return time.Date(y, m, day, h, mins, 0, 0, time.UTC)
	}
	cases := []struct {
		name string
		rule Rule
		from time.Time
		want time.Time
	}{
		{"daily +1", Rule{Freq: FreqDaily, Interval: 1}, d(2026, 6, 18, 9, 30), d(2026, 6, 19, 9, 30)},
		{"every 3 days", Rule{Freq: FreqDaily, Interval: 3}, d(2026, 6, 30, 0, 0), d(2026, 7, 3, 0, 0)},
		{"weekly", Rule{Freq: FreqWeekly, Interval: 1}, d(2026, 6, 18, 14, 0), d(2026, 6, 25, 14, 0)},
		{"every 2 weeks", Rule{Freq: FreqWeekly, Interval: 2}, d(2026, 6, 18, 14, 0), d(2026, 7, 2, 14, 0)},
		// The user's case: 30th monthly preserves time and clamps Feb.
		{"monthly 30th -> Feb clamp", Rule{Freq: FreqMonthly, Interval: 1, Day: 30}, d(2026, 1, 30, 18, 0), d(2026, 2, 28, 18, 0)},
		{"monthly 30th leap Feb", Rule{Freq: FreqMonthly, Interval: 1, Day: 30}, d(2024, 1, 30, 18, 0), d(2024, 2, 29, 18, 0)},
		// Anchor restores the 30th after a Feb clamp (would drift to 28 without it).
		{"monthly anchor restores 30th", Rule{Freq: FreqMonthly, Interval: 1, Day: 30}, d(2026, 2, 28, 18, 0), d(2026, 3, 30, 18, 0)},
		{"monthly no anchor uses from-day", Rule{Freq: FreqMonthly, Interval: 1}, d(2026, 3, 31, 8, 15), d(2026, 4, 30, 8, 15)},
		{"monthly across year", Rule{Freq: FreqMonthly, Interval: 1, Day: 15}, d(2026, 12, 15, 0, 0), d(2027, 1, 15, 0, 0)},
		{"every 2 months", Rule{Freq: FreqMonthly, Interval: 2, Day: 31}, d(2026, 1, 31, 0, 0), d(2026, 3, 31, 0, 0)},
		{"yearly", Rule{Freq: FreqYearly, Interval: 1, Day: 18, Month: 6}, d(2026, 6, 18, 12, 0), d(2027, 6, 18, 12, 0)},
		{"yearly Feb29 -> Feb28", Rule{Freq: FreqYearly, Interval: 1, Day: 29, Month: 2}, d(2024, 2, 29, 12, 0), d(2025, 2, 28, 12, 0)},
		// Yearly anchor restores Feb 29 in the next leap year.
		{"yearly anchor restores Feb29", Rule{Freq: FreqYearly, Interval: 1, Day: 29, Month: 2}, d(2025, 2, 28, 12, 0), d(2026, 2, 28, 12, 0)},
		{"yearly anchor leap target", Rule{Freq: FreqYearly, Interval: 3, Day: 29, Month: 2}, d(2025, 2, 28, 12, 0), d(2028, 2, 29, 12, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.rule.Next(c.from); !got.Equal(c.want) {
				t.Fatalf("Next=%s want %s", got, c.want)
			}
		})
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	r := Rule{Freq: FreqMonthly, Interval: 2, Day: 30}
	m, ok := r.Marshal()
	if !ok {
		t.Fatal("Marshal not ok")
	}
	got, ok := Parse(m)
	if !ok || got != r {
		t.Fatalf("round-trip=%+v,%v want %+v", got, ok, r)
	}
	if _, ok := (Rule{Freq: "bogus"}).Marshal(); ok {
		t.Fatal("Marshal of invalid rule should fail")
	}
}
