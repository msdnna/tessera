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

// ParseDate resolves a /due or /start argument to midnight of the target day in
// now's location. Accepted: 2026-08-14, 14.08.2026, 14.08 (current year),
// today/сегодня, tomorrow/завтра, yesterday/вчера, +3d, +2w, +1m.
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
			return day(now.AddDate(0, n, 0)), nil
		}
	}

	for _, layout := range []string{"2006-01-02", "02.01.2006", "02/01/2006"} {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			return t, nil
		}
	}
	// Day and month only — assume the current year.
	if t, err := time.ParseInLocation("02.01", s, now.Location()); err == nil {
		return time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()), nil
	}

	return time.Time{}, fmt.Errorf("не понимаю дату %q", raw)
}
