// Package recur describes how a recurring task advances when a trigger fires
// (the task is completed, moved to a column, or its scheduled time passes). The
// rule is stored as JSONB on the task and is intentionally provider-neutral; new
// fields can be added without a migration.
package recur

import (
	"encoding/json"
	"sort"
	"time"
)

// Frequencies.
const (
	FreqDaily   = "daily"
	FreqWeekly  = "weekly"
	FreqMonthly = "monthly"
	FreqYearly  = "yearly"
	FreqCustom  = "custom" // explicit calendar-picked dates
)

// Triggers — the event that advances the recurrence.
const (
	TriggerComplete = "complete" // task closed (done column / completed toggle)
	TriggerColumn   = "column"   // task moved into TriggerColumn
	TriggerSchedule = "schedule" // due date passes (background worker), no close needed
)

// dateLayout is the date-only form used by custom `dates`.
const dateLayout = "2006-01-02"

// Rule is a recurrence specification.
type Rule struct {
	Freq     string `json:"freq"`
	Interval int    `json:"interval"`
	// Weekly: which weekdays to repeat on (0=Sun … 6=Sat). Empty = same weekday
	// as the due date, every Interval weeks.
	Weekdays []int `json:"weekdays,omitempty"`
	// Custom: explicit sorted date-only occurrences ("2006-01-02").
	Dates []string `json:"dates,omitempty"`
	// Monthly/yearly anchor (server-managed) so the cadence survives a short-month
	// clamp — a task on the 30th stays the 30th even after Feb forces it to the 28th.
	Day   int `json:"day,omitempty"`   // 1..31
	Month int `json:"month,omitempty"` // 1..12 (yearly)
	// Trigger + routing.
	Trigger       string `json:"trigger,omitempty"`        // complete (default) | column | schedule
	TriggerColumn string `json:"trigger_column,omitempty"` // uuid, when trigger=column
	TargetColumn  string `json:"target_column,omitempty"`  // uuid, where the task lands (empty = first column)
	// Behaviour toggles.
	CreateNew    bool `json:"create_new,omitempty"`    // duplicate the task instead of rescheduling it
	Once         bool `json:"once,omitempty"`          // stop after one recurrence (default = forever)
	SkipWeekends bool `json:"skip_weekends,omitempty"` // daily/weekly: push occurrences off Sat/Sun
}

// Parse decodes and validates a raw recurrence document. ok is false for absent
// or malformed rules — callers treat those as "no recurrence".
func Parse(raw *json.RawMessage) (Rule, bool) {
	if raw == nil || len(*raw) == 0 {
		return Rule{}, false
	}
	var r Rule
	if err := json.Unmarshal(*raw, &r); err != nil {
		return Rule{}, false
	}
	return r.normalized()
}

// normalized validates the rule, clamps fields, and drops anything inapplicable
// to the chosen frequency.
func (r Rule) normalized() (Rule, bool) {
	switch r.Freq {
	case FreqDaily:
		r.Weekdays, r.Dates, r.Day, r.Month = nil, nil, 0, 0
	case FreqWeekly:
		r.Dates, r.Day, r.Month = nil, 0, 0
		r.Weekdays = cleanWeekdays(r.Weekdays)
	case FreqMonthly:
		r.Weekdays, r.Dates, r.Month = nil, nil, 0
	case FreqYearly:
		r.Weekdays, r.Dates = nil, nil
	case FreqCustom:
		r.Weekdays, r.Day, r.Month = nil, 0, 0
		r.Dates = cleanDates(r.Dates)
		if len(r.Dates) == 0 {
			return Rule{}, false
		}
	default:
		return Rule{}, false
	}
	if r.Interval < 1 {
		r.Interval = 1
	}
	if r.Day < 1 || r.Day > 31 {
		r.Day = 0
	}
	if r.Month < 1 || r.Month > 12 {
		r.Month = 0
	}
	switch r.Trigger {
	case TriggerComplete, TriggerColumn, TriggerSchedule:
	case "":
		r.Trigger = TriggerComplete
	default:
		r.Trigger = TriggerComplete
	}
	if r.Trigger != TriggerColumn {
		r.TriggerColumn = ""
	}
	// SkipWeekends only applies to day-stepping frequencies.
	if r.Freq != FreqDaily && r.Freq != FreqWeekly {
		r.SkipWeekends = false
	}
	return r, true
}

// cleanWeekdays sorts, de-dupes and range-checks a weekday list.
func cleanWeekdays(in []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, d := range in {
		if d >= 0 && d <= 6 && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Ints(out)
	return out
}

// cleanDates parses, de-dupes and sorts custom date strings, dropping invalid ones.
func cleanDates(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		t, err := time.Parse(dateLayout, s)
		if err != nil || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, t.Format(dateLayout))
	}
	sort.Strings(out)
	return out
}

// WithAnchor returns a copy with its monthly/yearly anchor set from the due date.
func (r Rule) WithAnchor(due time.Time) Rule {
	switch r.Freq {
	case FreqMonthly:
		r.Day = due.Day()
	case FreqYearly:
		r.Day = due.Day()
		r.Month = int(due.Month())
	}
	return r
}

// Marshal returns the canonical JSON form, suitable for storage.
func (r Rule) Marshal() (*json.RawMessage, bool) {
	n, ok := r.normalized()
	if !ok {
		return nil, false
	}
	b, err := json.Marshal(n)
	if err != nil {
		return nil, false
	}
	raw := json.RawMessage(b)
	return &raw, true
}

// Next returns the due date of the occurrence after `from`. ok is false when the
// recurrence has no further occurrence (a custom date list past its end). The
// time-of-day is preserved (custom dates inherit from's clock).
func (r Rule) Next(from time.Time) (time.Time, bool) {
	switch r.Freq {
	case FreqDaily:
		return r.applySkip(from.AddDate(0, 0, r.Interval)), true
	case FreqWeekly:
		return r.applySkip(r.nextWeekly(from)), true
	case FreqMonthly:
		total := int(from.Month()) - 1 + r.Interval
		return dateOn(from, from.Year()+total/12, time.Month(total%12+1), r.anchorDay(from)), true
	case FreqYearly:
		return dateOn(from, from.Year()+r.Interval, r.anchorMonth(from), r.anchorDay(from)), true
	case FreqCustom:
		return r.nextCustom(from)
	default:
		return from, false
	}
}

// NextAfter advances repeatedly until the occurrence is strictly after `bound`
// (used by the schedule worker to skip occurrences missed during downtime). ok is
// false if the recurrence ends before passing the bound.
func (r Rule) NextAfter(from, bound time.Time) (time.Time, bool) {
	cur := from
	for i := 0; i < 1000; i++ {
		next, ok := r.Next(cur)
		if !ok {
			return cur, false
		}
		if next.After(bound) {
			return next, true
		}
		cur = next
	}
	return cur, true
}

func (r Rule) anchorDay(from time.Time) int {
	if r.Day >= 1 {
		return r.Day
	}
	return from.Day()
}

func (r Rule) anchorMonth(from time.Time) time.Month {
	if r.Month >= 1 {
		return time.Month(r.Month)
	}
	return from.Month()
}

// nextWeekly handles weekday-set repeats (every Interval weeks on the chosen
// weekdays); with no weekday set it simply steps Interval whole weeks.
func (r Rule) nextWeekly(from time.Time) time.Time {
	if len(r.Weekdays) == 0 {
		return from.AddDate(0, 0, 7*r.Interval)
	}
	set := map[int]bool{}
	for _, d := range r.Weekdays {
		set[d] = true
	}
	// A later selected weekday in the same Mon-started week?
	monday := mondayOf(from)
	for d := from.AddDate(0, 0, 1); !d.Before(monday) && d.Before(monday.AddDate(0, 0, 7)); d = d.AddDate(0, 0, 1) {
		if set[int(d.Weekday())] {
			return d
		}
	}
	// Otherwise jump Interval weeks ahead to the first selected weekday.
	target := monday.AddDate(0, 0, 7*r.Interval)
	for i := 0; i < 7; i++ {
		d := target.AddDate(0, 0, i)
		if set[int(d.Weekday())] {
			return d
		}
	}
	return from.AddDate(0, 0, 7*r.Interval)
}

// nextCustom returns the first custom date strictly after from's date.
func (r Rule) nextCustom(from time.Time) (time.Time, bool) {
	key := from.Format(dateLayout)
	for _, s := range r.Dates {
		if s > key {
			d, err := time.Parse(dateLayout, s)
			if err != nil {
				continue
			}
			hh, mm, ss := from.Clock()
			return time.Date(d.Year(), d.Month(), d.Day(), hh, mm, ss, from.Nanosecond(), from.Location()), true
		}
	}
	return from, false
}

// applySkip pushes a Saturday/Sunday occurrence forward to Monday when the rule
// excludes weekends. No-op when SkipWeekends is off.
func (r Rule) applySkip(t time.Time) time.Time {
	if !r.SkipWeekends {
		return t
	}
	switch t.Weekday() {
	case time.Saturday:
		return t.AddDate(0, 0, 2)
	case time.Sunday:
		return t.AddDate(0, 0, 1)
	default:
		return t
	}
}

// mondayOf returns midnight-agnostic Monday of t's week (keeps clock/location).
func mondayOf(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7 // Mon=0 … Sun=6
	return t.AddDate(0, 0, -offset)
}

// dateOn builds a time on year/month/day (day clamped to the month length),
// carrying base's clock and location.
func dateOn(base time.Time, year int, month time.Month, day int) time.Time {
	if last := daysInMonth(year, month); day > last {
		day = last
	}
	hh, mm, ss := base.Clock()
	return time.Date(year, month, day, hh, mm, ss, base.Nanosecond(), base.Location())
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
