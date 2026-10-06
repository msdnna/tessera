// A refused estimate must leave the task's estimate alone. That is not what
// happened: quickact.ParseEstimate had two ways of returning a number no column
// can hold without returning an error with it — "nan"/"inf" spelled out on a
// points board (strconv.ParseFloat reads both as floats), and float64 overflow
// on a time one. handlers.normalizeEstimate then mapped the non-finite value to
// nil, and nil on a patch means *clear the estimate* — so the comment came back
// with «оценка nan» under «применено» while the number the task really carried
// was gone (#2932).
//
// Both halves are checked here rather than in internal/quickact alone, because
// the damage is in the handoff: the parser's return value and the patch layer's
// idea of what nil means are only wrong together.
package main

import (
	"net/http"
	"strings"
	"testing"
)

// taskEstimate reads a task's stored estimate, or -1 when it carries none.
func taskEstimate(t *testing.T, c *client, taskID string) float64 {
	t.Helper()
	task := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	v, ok := task["estimate"].(float64)
	if !ok {
		return -1
	}
	return v
}

// runEstimate posts an /estimate command and reports whether it was applied.
//
// 400 is one of the expected answers, not a failure: a comment that is nothing
// but commands and whose only command was refused applies nothing, and the
// route says so. The summary it carries is still the thing under test.
func runEstimate(t *testing.T, c *client, taskID, arg string) bool {
	t.Helper()
	r := c.post("/tasks/"+taskID+"/comments", map[string]any{"body": "/estimate " + arg})
	switch r.Status {
	case http.StatusOK, http.StatusCreated, http.StatusBadRequest:
	default:
		t.Fatalf("status %d for /estimate %.20s…\n%s", r.Status, arg, r.Body)
	}
	applied, _ := commandSummary(t, r)
	for _, key := range applied {
		if key == "estimate" {
			return true
		}
	}
	return false
}

func TestQuickActionEstimateOverflowKeepsEstimate(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	taskID := mkTask(t, c, s.Board, s.col(t, 0), "Задача с оценкой")["id"].(string)

	if runEstimate(t, c, taskID, "2ч") != true {
		t.Fatal("/estimate 2ч was not applied")
	}
	if got := taskEstimate(t, c, taskID); got != 120 {
		t.Fatalf("оценка = %v, ожидалось 120 минут", got)
	}

	// 3e306 hours: the number parses, ×60 leaves float64 range.
	huge := "3" + strings.Repeat("0", 306) + "ч"
	if runEstimate(t, c, taskID, huge) {
		t.Error("переполненная оценка попала в «применено»")
	}
	if got := taskEstimate(t, c, taskID); got != 120 {
		t.Fatalf("оценка после отказа = %v, ожидалось прежние 120 минут", got)
	}
}

func TestQuickActionEstimateNaNKeepsEstimate(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	// A points workspace reads the argument as a plain number, which is the path
	// that went straight to strconv.ParseFloat — and it accepts "nan" and "inf".
	c.expect(t, c.put("/workspaces/"+s.WS+"/estimation",
		map[string]any{"unit": "points", "points_scale": "linear"}), http.StatusOK)
	taskID := mkTask(t, c, s.Board, s.col(t, 0), "Задача в сторипойнтах")["id"].(string)

	if !runEstimate(t, c, taskID, "5") {
		t.Fatal("/estimate 5 was not applied")
	}
	if got := taskEstimate(t, c, taskID); got != 5 {
		t.Fatalf("оценка = %v, ожидалось 5 поинтов", got)
	}

	for _, arg := range []string{"nan", "inf", "infinity"} {
		if runEstimate(t, c, taskID, arg) {
			t.Errorf("/estimate %s попала в «применено»", arg)
		}
		if got := taskEstimate(t, c, taskID); got != 5 {
			t.Fatalf("оценка после «/estimate %s» = %v, ожидалось прежние 5", arg, got)
		}
	}
}
