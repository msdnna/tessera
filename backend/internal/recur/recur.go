// Package recur describes how a recurring task's due date advances when the task
// is closed. The rule is stored as JSONB on the task and is intentionally small
// and provider-neutral; new fields (weekday masks, end conditions) can be added
// without a migration.
package recur

import (
	"encoding/json"
	"time"
)

// Allowed frequencies.
const (
	FreqDaily   = "daily"
	FreqWeekly  = "weekly"
	FreqMonthly = "monthly"
	FreqYearly  = "yearly"
)

// Rule is a recurrence specification: every Interval Freq-units. Day/Month are a
// backend-managed anchor for monthly/yearly rules so the cadence survives the
// short-month clamp: a task due on the 30th stays anchored to the 30th even after
// February forces it to the 28th. Clients send only {freq, interval}; the server
// fills the anchor from the due date.
type Rule struct {
	Freq     string `json:"freq"`
	Interval int    `json:"interval"`
	Day      int    `json:"day,omitempty"`   // anchor day-of-month (1..31), monthly/yearly
	Month    int    `json:"month,omitempty"` // anchor month (1..12), yearly only
}

// Parse decodes and validates a raw recurrence document. ok is false for absent
// or malformed rules, an unknown frequency, or a non-positive interval — callers
// treat those as "no recurrence".
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

// normalized validates the rule, clamps the interval to at least 1, and drops
// out-of-range or inapplicable anchor fields.
func (r Rule) normalized() (Rule, bool) {
	switch r.Freq {
	case FreqDaily, FreqWeekly:
		r.Day, r.Month = 0, 0
	case FreqMonthly:
		r.Month = 0
	case FreqYearly:
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
	return r, true
}

// WithAnchor returns a copy of the rule with its monthly/yearly anchor set from
// the given due date — capturing the day-of-month (and month, for yearly) the
// user intends to repeat on.
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

// Marshal returns the canonical JSON form of a rule, suitable for storage.
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

// Next returns the due date one period after `from`. Daily/weekly add days;
// monthly/yearly land on the anchor day (falling back to from's day when no
// anchor is set), clamped to the target month's last day — so a task anchored to
// the 30th lands on Feb 28/29 but bounces back to the 30th in March, and never
// overflows into the next month the way time.AddDate would. Time-of-day is always
// preserved.
func (r Rule) Next(from time.Time) time.Time {
	switch r.Freq {
	case FreqDaily:
		return from.AddDate(0, 0, r.Interval)
	case FreqWeekly:
		return from.AddDate(0, 0, 7*r.Interval)
	case FreqMonthly:
		total := int(from.Month()) - 1 + r.Interval
		return dateOn(from, from.Year()+total/12, time.Month(total%12+1), r.anchorDay(from))
	case FreqYearly:
		return dateOn(from, from.Year()+r.Interval, r.anchorMonth(from), r.anchorDay(from))
	default:
		return from
	}
}

// anchorDay is the day-of-month to repeat on: the stored anchor, else from's day.
func (r Rule) anchorDay(from time.Time) int {
	if r.Day >= 1 {
		return r.Day
	}
	return from.Day()
}

// anchorMonth is the month to repeat on for yearly rules: the stored anchor, else
// from's month.
func (r Rule) anchorMonth(from time.Time) time.Month {
	if r.Month >= 1 {
		return time.Month(r.Month)
	}
	return from.Month()
}

// dateOn builds a time on the given year/month/day (day clamped to the month's
// length), carrying base's clock and location.
func dateOn(base time.Time, year int, month time.Month, day int) time.Time {
	if last := daysInMonth(year, month); day > last {
		day = last
	}
	hh, mm, ss := base.Clock()
	return time.Date(year, month, day, hh, mm, ss, base.Nanosecond(), base.Location())
}

// daysInMonth returns the number of days in the given year/month.
func daysInMonth(year int, month time.Month) int {
	// Day 0 of the next month is the last day of this one.
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
