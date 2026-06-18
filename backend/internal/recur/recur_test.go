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
		{"daily default trigger", raw(`{"freq":"daily","interval":3}`), true, Rule{Freq: FreqDaily, Interval: 3, Trigger: TriggerComplete}},
		{"interval clamp", raw(`{"freq":"weekly","interval":0}`), true, Rule{Freq: FreqWeekly, Interval: 1, Trigger: TriggerComplete}},
		{"monthly anchor", raw(`{"freq":"monthly","interval":1,"day":30}`), true, Rule{Freq: FreqMonthly, Interval: 1, Day: 30, Trigger: TriggerComplete}},
		{"daily drops anchor + weekdays", raw(`{"freq":"daily","interval":1,"day":30,"weekdays":[1]}`), true, Rule{Freq: FreqDaily, Interval: 1, Trigger: TriggerComplete}},
		{"weekly weekdays sorted/deduped", raw(`{"freq":"weekly","interval":1,"weekdays":[3,1,1,7]}`), true, Rule{Freq: FreqWeekly, Interval: 1, Weekdays: []int{1, 3}, Trigger: TriggerComplete}},
		{"custom dates sorted", raw(`{"freq":"custom","dates":["2026-06-20","2026-06-10"]}`), true, Rule{Freq: FreqCustom, Interval: 1, Dates: []string{"2026-06-10", "2026-06-20"}, Trigger: TriggerComplete}},
		{"custom empty invalid", raw(`{"freq":"custom","dates":[]}`), false, Rule{}},
		{"column trigger", raw(`{"freq":"daily","interval":1,"trigger":"column","trigger_column":"abc","target_column":"def","create_new":true,"once":true,"skip_weekends":true}`), true,
			Rule{Freq: FreqDaily, Interval: 1, Trigger: TriggerColumn, TriggerColumn: "abc", TargetColumn: "def", CreateNew: true, Once: true, SkipWeekends: true}},
		{"skip_weekends dropped for monthly", raw(`{"freq":"monthly","interval":1,"skip_weekends":true}`), true, Rule{Freq: FreqMonthly, Interval: 1, Trigger: TriggerComplete}},
		{"trigger_column dropped when not column", raw(`{"freq":"daily","interval":1,"trigger":"complete","trigger_column":"abc"}`), true, Rule{Freq: FreqDaily, Interval: 1, Trigger: TriggerComplete}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Parse(c.in)
			if ok != c.wantOK {
				t.Fatalf("Parse ok=%v want %v", ok, c.wantOK)
			}
			if ok && !equalRule(got, c.wantR) {
				t.Fatalf("Parse=%+v want %+v", got, c.wantR)
			}
		})
	}
}

func equalRule(a, b Rule) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func d(y int, m time.Month, day, h, mins int) time.Time {
	return time.Date(y, m, day, h, mins, 0, 0, time.UTC)
}

func TestNext(t *testing.T) {
	cases := []struct {
		name   string
		rule   Rule
		from   time.Time
		want   time.Time
		wantOK bool
	}{
		{"daily +1", Rule{Freq: FreqDaily, Interval: 1}, d(2026, 6, 18, 9, 30), d(2026, 6, 19, 9, 30), true},
		{"every 3 days", Rule{Freq: FreqDaily, Interval: 3}, d(2026, 6, 30, 0, 0), d(2026, 7, 3, 0, 0), true},
		// skip weekends: Fri+1 = Sat → Mon.
		{"daily skip weekend", Rule{Freq: FreqDaily, Interval: 1, SkipWeekends: true}, d(2026, 6, 19, 9, 0), d(2026, 6, 22, 9, 0), true},
		{"weekly stride", Rule{Freq: FreqWeekly, Interval: 1}, d(2026, 6, 18, 14, 0), d(2026, 6, 25, 14, 0), true},
		{"every 2 weeks", Rule{Freq: FreqWeekly, Interval: 2}, d(2026, 6, 18, 14, 0), d(2026, 7, 2, 14, 0), true},
		// 2026-06-18 is a Thursday. weekdays Mon(1),Wed(3),Fri(5): next after Thu = Fri same week.
		{"weekly weekdays same week", Rule{Freq: FreqWeekly, Interval: 1, Weekdays: []int{1, 3, 5}}, d(2026, 6, 18, 10, 0), d(2026, 6, 19, 10, 0), true},
		// From Fri 2026-06-19 with Mon/Wed/Fri, interval 1 → next is Mon 2026-06-22.
		{"weekly weekdays wrap", Rule{Freq: FreqWeekly, Interval: 1, Weekdays: []int{1, 3, 5}}, d(2026, 6, 19, 10, 0), d(2026, 6, 22, 10, 0), true},
		// interval 2, from Fri 06-19 (Mon/Fri); week starts Mon 06-15, +2 weeks = Mon 06-29.
		{"weekly weekdays interval2 jump", Rule{Freq: FreqWeekly, Interval: 2, Weekdays: []int{1, 5}}, d(2026, 6, 19, 10, 0), d(2026, 6, 29, 10, 0), true},
		// The user's case: monthly anchor 30 → Feb clamp → restores in March.
		{"monthly 30 -> Feb", Rule{Freq: FreqMonthly, Interval: 1, Day: 30}, d(2026, 1, 30, 18, 0), d(2026, 2, 28, 18, 0), true},
		{"monthly anchor restores 30", Rule{Freq: FreqMonthly, Interval: 1, Day: 30}, d(2026, 2, 28, 18, 0), d(2026, 3, 30, 18, 0), true},
		{"yearly", Rule{Freq: FreqYearly, Interval: 1, Day: 18, Month: 6}, d(2026, 6, 18, 12, 0), d(2027, 6, 18, 12, 0), true},
		{"yearly Feb29 restores", Rule{Freq: FreqYearly, Interval: 3, Day: 29, Month: 2}, d(2025, 2, 28, 12, 0), d(2028, 2, 29, 12, 0), true},
		// custom: next date after from, time preserved.
		{"custom next", Rule{Freq: FreqCustom, Dates: []string{"2026-06-10", "2026-06-20", "2026-07-01"}}, d(2026, 6, 10, 9, 0), d(2026, 6, 20, 9, 0), true},
		{"custom exhausted", Rule{Freq: FreqCustom, Dates: []string{"2026-06-10"}}, d(2026, 6, 10, 9, 0), time.Time{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := c.rule.Next(c.from)
			if ok != c.wantOK {
				t.Fatalf("Next ok=%v want %v", ok, c.wantOK)
			}
			if ok && !got.Equal(c.want) {
				t.Fatalf("Next=%s want %s", got, c.want)
			}
		})
	}
}

func TestNextAfter(t *testing.T) {
	// Monthly on the 15th, last fired in January; server was down until April →
	// should skip to the next occurrence strictly after the bound (May 15).
	r := Rule{Freq: FreqMonthly, Interval: 1, Day: 15}
	got, ok := r.NextAfter(d(2026, 1, 15, 9, 0), d(2026, 4, 20, 0, 0))
	if !ok || !got.Equal(d(2026, 5, 15, 9, 0)) {
		t.Fatalf("NextAfter=%s ok=%v want 2026-05-15", got, ok)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	r := Rule{Freq: FreqWeekly, Interval: 2, Weekdays: []int{1, 4}, Trigger: TriggerColumn, TriggerColumn: "x", CreateNew: true}
	m, ok := r.Marshal()
	if !ok {
		t.Fatal("Marshal not ok")
	}
	got, ok := Parse(m)
	if !ok || !equalRule(got, r) {
		t.Fatalf("round-trip=%+v want %+v", got, r)
	}
	if _, ok := (Rule{Freq: "bogus"}).Marshal(); ok {
		t.Fatal("Marshal of invalid rule should not be ok")
	}
}
