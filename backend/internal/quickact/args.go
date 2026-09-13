package quickact

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// priorityWords maps what a user can type to tasks.priority (0..4), matching
// PRIORITY_LABELS in the frontend (styles/tokens.js).
var priorityWords = map[string]int32{
	"0": 0, "нет": 0, "без": 0, "без приоритета": 0, "none": 0,
	"1": 1, "низкий": 1, "low": 1,
	"2": 2, "обычный": 2, "средний": 2, "normal": 2, "medium": 2,
	"3": 3, "высокий": 3, "high": 3,
	"4": 4, "срочный": 4, "urgent": 4, "critical": 4,
}

// ParsePriority resolves a /priority argument to 0..4, by word or by digit.
func ParsePriority(raw string) (int32, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if p, ok := priorityWords[s]; ok {
		return p, nil
	}
	return 0, fmt.Errorf("не понимаю приоритет %q (нет/низкий/обычный/высокий/срочный)", raw)
}

// estimateToken matches one "2h" / "30м" / "1.5d" chunk of an estimate.
var estimateToken = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*([wнdдhчmм]?)`)

// Estimation is the estimate configuration in force for a task (project
// override → workspace default → the built-in time/8h/5d), reduced to what
// argument parsing needs. The zero value means the built-in default.
type Estimation struct {
	Unit        string  // "time" | "points" | "custom"; empty = time
	HoursPerDay float64 // time only; ≤0 = 8
	DaysPerWeek float64 // time only; ≤0 = 5
	PointsScale string  // points only: fibonacci | tshirt | linear
}

// tshirtSizes mirrors POINTS_SCALES.tshirt in the clients (frontend
// utils/estimation.js, android util/Estimation.kt): the size is what a user
// types and sees, the number is what gets stored.
var tshirtSizes = map[string]float64{"xs": 1, "s": 2, "m": 3, "l": 5, "xl": 8, "xxl": 13}

// ParseEstimate resolves an /estimate argument to the stored value, reading it
// exactly the way the estimate field in the clients reads its input — a comment
// and a popover that set the same number must not disagree about what it means.
//
// In a time workspace the result is minutes: with a unit suffix (2h30m, 1d, 1w,
// Russian ч/д/н/м) the day/week arithmetic uses the workspace settings, and a
// bare number is HOURS, as in `parseEstimate` on both clients. (It used to be
// minutes here and only here, so "/estimate 3" stored three minutes where the
// popover would have stored three hours — the same typing, off by sixty.)
//
// Points and custom units count something dimensionless, so there the argument
// is a plain number — plus the T-shirt sizes when the scale is tshirt. A
// duration is refused rather than counted: "/estimate 2h" on a points board
// used to store 120 points.
func ParseEstimate(raw string, cfg Estimation) (float64, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0, fmt.Errorf("не указана оценка")
	}
	if cfg.Unit != "" && cfg.Unit != "time" {
		return parseDimensionless(s, raw, cfg)
	}

	hoursPerDay, daysPerWeek := cfg.HoursPerDay, cfg.DaysPerWeek
	if hoursPerDay <= 0 {
		hoursPerDay = 8
	}
	if daysPerWeek <= 0 {
		daysPerWeek = 5
	}

	matches := estimateToken.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return 0, fmt.Errorf("не понимаю оценку %q", raw)
	}
	// Everything the tokens did not consume is junk ("2h кое-что").
	if strings.TrimSpace(estimateToken.ReplaceAllString(s, "")) != "" {
		return 0, fmt.Errorf("не понимаю оценку %q", raw)
	}

	var total float64
	var unitSeen bool
	for _, m := range matches {
		n, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
		if err != nil {
			return 0, fmt.Errorf("не понимаю оценку %q", raw)
		}
		switch m[2] {
		case "":
			total += n * 60 // bare number: hours, as in the clients' estimate field
		case "m", "м":
			unitSeen = true
			total += n
		case "h", "ч":
			unitSeen = true
			total += n * 60
		case "d", "д":
			unitSeen = true
			total += n * hoursPerDay * 60
		default: // w, н
			unitSeen = true
			total += n * daysPerWeek * hoursPerDay * 60
		}
	}
	if len(matches) > 1 && !unitSeen {
		return 0, fmt.Errorf("не понимаю оценку %q", raw)
	}
	if total <= 0 {
		return 0, fmt.Errorf("оценка должна быть больше нуля")
	}
	return total, nil
}

// parseDimensionless reads the argument of a points / custom-unit workspace: a
// T-shirt size on a tshirt scale, otherwise a plain number. `s` is the
// lowercased form, `raw` the original, kept for the error text.
func parseDimensionless(s, raw string, cfg Estimation) (float64, error) {
	if cfg.Unit == "points" && cfg.PointsScale == "tshirt" {
		if v, ok := tshirtSizes[s]; ok {
			return v, nil
		}
	}
	n, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	if err != nil {
		return 0, fmt.Errorf("не понимаю оценку %q (нужно число)", raw)
	}
	if n <= 0 {
		return 0, fmt.Errorf("оценка должна быть больше нуля")
	}
	return n, nil
}

var taskRef = regexp.MustCompile(`#?(\d{1,9})`)

// ParseRefs pulls task numbers out of a relation argument ("#123", "123").
func ParseRefs(raw string) ([]int32, error) {
	matches := taskRef.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("не указан номер задачи (например #123)")
	}
	seen := map[int32]bool{}
	out := make([]int32, 0, len(matches))
	for _, m := range matches {
		n, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil || n <= 0 {
			continue
		}
		if num := int32(n); !seen[num] {
			seen[num] = true
			out = append(out, num)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("не указан номер задачи (например #123)")
	}
	return out, nil
}

var mention = regexp.MustCompile(`@([^\s,@]+)`)

// ParseMentions pulls @logins out of an /assign argument. Resolution against
// workspace members is the handler's job.
func ParseMentions(raw string) []string {
	matches := mention.FindAllStringSubmatch(raw, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		name := strings.ToLower(strings.Trim(m[1], ".,;:!?"))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// ParseTags splits a /tag argument on commas, keeping names with spaces intact.
func ParseTags(raw string) []string {
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}
