package quickact

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// relDate matches a relative offset: +3d, -2w, +1m (Russian д/н/м too).
var relDate = regexp.MustCompile(`^([+-])(\d{1,4})\s*([dwmднм])$`)

// dmyDate matches a day-first date with an optional year: 14.08.2026, 1.9,
// 04/09/2026. Both separators are accepted in both positions because a date is
// typed, not picked, and nobody re-reads which one the hint used.
var dmyDate = regexp.MustCompile(`^(\d{1,2})[./](\d{1,2})(?:[./](\d{4}))?$`)

// isoDate matches the ISO form, month and day unpadded too: 2026-8-14.
var isoDate = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)

// ParseDate resolves a /due or /start argument to midnight of the target day in
// now's location. Accepted: 2026-08-14, 14.08.2026, 14.08 (current year),
// today/сегодня, tomorrow/завтра, yesterday/вчера, +3d, +2w, +1m.
//
// The numeric forms are matched by shape and re-assembled into a padded layout
// rather than handed to time.Parse as typed: Go's "02" and "01" are fixed-width
// and reject a single digit, so "1.9" — the ordinary way to write a date in a
// hurry — came back as «не понимаю дату» while "01.09" worked. Padding here
// keeps the range checking where it was (31.02 is still an error, because the
// padded string is still parsed by time.ParseInLocation).
//
// Errors are user-facing: they land in the comment's command summary.
func ParseDate(raw string, now time.Time) (time.Time, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return time.Time{}, fmt.Errorf("не указана дата")
	}
	day := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
	}

	switch s {
	case "today", "сегодня":
		return day(now), nil
	case "tomorrow", "завтра":
		return day(now.AddDate(0, 0, 1)), nil
	case "yesterday", "вчера":
		return day(now.AddDate(0, 0, -1)), nil
	}

	if m := relDate.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[2])
		if err != nil { // unreachable: the pattern only matches digits
			return time.Time{}, fmt.Errorf("не понимаю дату %q", raw)
		}
		if m[1] == "-" {
			n = -n
		}
		switch m[3] {
		case "d", "д":
			return day(now.AddDate(0, 0, n)), nil
		case "w", "н":
			return day(now.AddDate(0, 0, n*7)), nil
		default: // m, м
			return addMonths(day(now), n), nil
		}
	}

	if m := isoDate.FindStringSubmatch(s); m != nil {
		return parseYMD(m[1], m[2], m[3], now, raw)
	}
	if m := dmyDate.FindStringSubmatch(s); m != nil {
		year := m[3]
		if year == "" { // day and month only — assume the current year
			year = strconv.Itoa(now.Year())
		}
		return parseYMD(year, m[2], m[1], now, raw)
	}

	return time.Time{}, fmt.Errorf("не понимаю дату %q", raw)
}

// parseYMD assembles the three captured numbers into the padded ISO layout and
// lets time do the validating — so an impossible day (31.02, month 13) is still
// the error it always was, and midnight is still midnight in now's zone.
func parseYMD(year, month, day string, now time.Time, raw string) (time.Time, error) {
	y, errY := strconv.Atoi(year)
	mo, errM := strconv.Atoi(month)
	d, errD := strconv.Atoi(day)
	if errY != nil || errM != nil || errD != nil { // unreachable: the patterns only match digits
		return time.Time{}, fmt.Errorf("не понимаю дату %q", raw)
	}
	padded := fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
	t, err := time.ParseInLocation("2006-01-02", padded, now.Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("не понимаю дату %q", raw)
	}
	return t, nil
}

// addMonths steps n whole calendar months from t, clamping the day to the target
// month's length.
//
// time.AddDate normalises instead of clamping, and "+1m" written on the 31st then
// lands on a month the person never named: 31 января +1м overflowed 31 февраля into
// 3 марта, skipping February outright. Backwards it is worse — 31 марта -1м came out
// as 3 марта, so "минус месяц" moved 28 days back and left the month unchanged. The
// recurrence engine clamps for exactly this reason (internal/recur dateOn).
func addMonths(t time.Time, n int) time.Time {
	// Year is always positive here, so plain / and % stay floor-correct even for n<0.
	total := t.Year()*12 + int(t.Month()) - 1 + n
	year, month := total/12, time.Month(total%12+1)
	d := t.Day()
	if last := daysInMonth(year, month); d > last {
		d = last
	}
	return time.Date(year, month, d, 0, 0, 0, 0, t.Location())
}

// daysInMonth is the length of year/month — day 0 of the next month is the last
// day of this one.
func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
