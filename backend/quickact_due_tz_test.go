package main

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// «/due сегодня» has to mean the day the person writing the comment is living
// through, not the one the server process happens to be in. The clients store a
// due date as the local midnight the user picked (see resolvedTimezone in
// frontend/src/utils/format.js), and they colour the card by their own calendar
// day — so a command resolved against the server's midnight lands on the wrong
// date twice over: the stored instant is a different day's start, and the summary
// the command writes back into the comment ("срок 14.08.2026") then contradicts
// the due pill the frontend renders on that very card.
//
// Asserting against the caller's midnight is what keeps this independent of
// wherever the test host runs, and two zones 26 hours apart are enough that at
// most one of them can coincide with the server's own day — the same argument
// TestWorkspaceSummaryBucketsByCallerTimezone makes for the home tiles.
func TestQuickActionDueResolvesInCallerTimezone(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	for _, zone := range []string{"Etc/GMT-14", "Etc/GMT+12"} {
		want := localMidnight(t, zone)
		q := "?tz=" + url.QueryEscape(zone)

		task := mkTask(t, c, s.Board, s.col(t, 0), "срок из команды в "+zone)
		taskID := task["id"].(string)

		// A body that is nothing but commands answers 200 and leaves no comment.
		c.expect(t, c.post("/tasks/"+taskID+"/comments"+q,
			map[string]any{"body": "/due сегодня"}), http.StatusOK)

		after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
		got := parseDueDate(t, after["due_date"])
		if !got.Equal(want) {
			t.Errorf("tz=%s: due_date = %s, want the caller's midnight %s",
				zone, got.Format(time.RFC3339), want.Format(time.RFC3339))
		}

		// The summary is what the user reads back in the comment; it has to name the
		// same day the pill on the card shows.
		m := c.expect(t, c.post("/tasks/"+taskID+"/commands/preview"+q,
			map[string]any{"body": "/due сегодня"}), http.StatusOK)
		if summary := firstCommandSummary(t, m); summary != "срок "+want.Format("02.01.2006") {
			t.Errorf("tz=%s: preview summary = %q, want %q",
				zone, summary, "срок "+want.Format("02.01.2006"))
		}
	}
}

// Without a zone on the request the command falls back to the zone stored in the
// profile — the same preference the notification scanner evaluates quiet hours
// against, and the same fallback the home summary uses.
func TestQuickActionDueFallsBackToStoredTimezone(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	const zone = "Etc/GMT-14"
	c.expect(t, c.put("/users/me/preferences", map[string]any{
		"language": "ru", "timezone": zone, "country": "",
		"time_format": "24h", "date_format": "dd.MM.yyyy", "week_start": 1,
		"theme": "system", "accent": "purple", "board_background": "",
	}), http.StatusOK)

	want := localMidnight(t, zone)
	task := mkTask(t, c, s.Board, s.col(t, 0), "срок по зоне из профиля")
	taskID := task["id"].(string)

	c.expect(t, c.post("/tasks/"+taskID+"/comments",
		map[string]any{"body": "/start сегодня"}), http.StatusOK)

	after := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	got := parseDueDate(t, after["start_date"])
	if !got.Equal(want) {
		t.Errorf("start_date = %s, want the profile zone's midnight %s",
			got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

// parseDueDate reads a date field off a task payload.
func parseDueDate(t *testing.T, raw any) time.Time {
	t.Helper()
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("date field = %v (%T), want an RFC3339 string", raw, raw)
	}
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return v
}

// firstCommandSummary pulls the summary of the first previewed command.
func firstCommandSummary(t *testing.T, m map[string]any) string {
	t.Helper()
	cmds, _ := m["commands"].([]any)
	if len(cmds) != 1 {
		t.Fatalf("preview returned %d commands, want 1: %v", len(cmds), m)
	}
	first, _ := cmds[0].(map[string]any)
	if first["error"] != nil {
		t.Fatalf("preview errored: %v", first)
	}
	s, _ := first["summary"].(string)
	return s
}
