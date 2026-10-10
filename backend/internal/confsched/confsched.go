// Package confsched answers one question about a planned meeting (#3052): when
// is it next?
//
// A conference is a reusable room (#2879) — a daily standup is one conference,
// not a row per day — so a repeat is not a series of copies. It is a rule on the
// one row, and the moment the list shows is computed from the planned start, the
// rule, the time zone and the length whenever the row is read. Nothing advances
// in the database, so there is no worker to miss a tick and no stale row after a
// restart.
//
// The rules are the mockup's presets (MeetPage «не повторять / по будням /
// каждые 2 недели»), which is the whole set a meeting needs; they are kept apart
// from the task recurrence engine (internal/recur), whose triggers and subtask
// carry-over have no meaning for a call.
package confsched

import (
	"time"
)

// The rules. None ("") means a one-off meeting.
const (
	None     = ""
	Daily    = "daily"
	Weekdays = "weekdays"
	Weekly   = "weekly"
	Biweekly = "biweekly"
	Monthly  = "monthly"
)

// MaxDuration caps a meeting's length in minutes — the CHECK on
// conferences.duration_minutes says the same.
const MaxDuration = 24 * 60

// Valid reports whether rule is one of the presets (or none).
func Valid(rule string) bool {
	switch rule {
	case None, Daily, Weekdays, Weekly, Biweekly, Monthly:
		return true
	}
	return false
}

// Location resolves a stored zone name; an empty or unknown one is UTC — the
// same fallback the rest of the server uses for a profile without a zone.
func Location(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.UTC
}

// Next returns the occurrence a reader should be shown at `now`: the first one
// that has not finished yet. A meeting in progress (started, its length not over)
// is still "next" — the list keeps today's 17:30 standup until 17:45, not jumps to
// tomorrow at 17:31. Without a length an occurrence is over the moment it starts.
//
// Occurrences keep the wall-clock time of `start` in `loc`: «по будням в 11:00»
// stays 11:00 after a daylight-saving switch, which a fixed 24h step would not.
// A one-off meeting is returned as is, past or not — that is a plan that was
// never held, and the list says so.
func Next(start time.Time, rule string, loc *time.Location, length time.Duration, now time.Time) time.Time {
	if rule == None || !Valid(rule) {
		return start
	}
	if loc == nil {
		loc = time.UTC
	}
	if length < 0 {
		length = 0
	}
	over := func(t time.Time) bool { return !t.Add(length).After(now) }
	if !over(start) {
		return start
	}
	base := start.In(loc)
	switch rule {
	case Daily, Weekly, Biweekly:
		step := map[string]int{Daily: 1, Weekly: 7, Biweekly: 14}[rule]
		// Jump close to `now` in whole steps, then walk: a standup planned two
		// years ago must not loop seven hundred times on every list read.
		days := daysBetween(base, now.In(loc))
		k := days/step - 1
		if k < 0 {
			k = 0
		}
		for ; ; k++ {
			t := base.AddDate(0, 0, k*step)
			if !over(t) {
				return t
			}
		}
	case Weekdays:
		days := daysBetween(base, now.In(loc)) - 3
		if days < 0 {
			days = 0
		}
		for k := days; ; k++ {
			t := base.AddDate(0, 0, k)
			if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
				continue
			}
			if !over(t) {
				return t
			}
		}
	case Monthly:
		months := (now.In(loc).Year()-base.Year())*12 + int(now.In(loc).Month()-base.Month()) - 1
		if months < 0 {
			months = 0
		}
		for k := months; ; k++ {
			t := addMonthsClamped(base, k)
			if !over(t) {
				return t
			}
		}
	}
	return start
}

// daysBetween counts calendar days from a to b in a's location.
func daysBetween(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.In(a.Location()).Date()
	da := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	db := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}

// addMonthsClamped moves t by n months, keeping the day of month where the
// target month has it and the last day where it does not: a meeting on the 31st
// lands on the 30th of April, not on the 1st of May (Go's AddDate would roll
// over).
func addMonthsClamped(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	last := first.AddDate(0, 1, -1).Day()
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}
