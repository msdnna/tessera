package main

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// The home tiles are bucketed by calendar day, and the day that matters is the
// caller's — not the one the server process happens to live in. A user in +03:00
// picks "today" in the due editor and the client stores that day's local midnight,
// i.e. 21:00 of the previous day in UTC; a server counting against its own midnight
// then files that task under "Просрочено" for the whole day, while the row right
// under the tile is not red (HomeView.vue derives its own state from the browser's
// calendar day). The two must agree — see the note in workspace_tasks.sql.
//
// Asserting on the boundary itself is what makes this independent of wherever the
// test host runs: a task at the caller's midnight has to be "today" and one a
// second earlier has to be "overdue", and both hold only if the cut is exactly the
// caller's midnight. Two zones 26 hours apart are then enough — their midnights
// land two hours apart, so at most one of them can coincide with the server's.
func TestWorkspaceSummaryBucketsByCallerTimezone(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	for _, zone := range []string{"Etc/GMT-14", "Etc/GMT+12"} {
		midnight := localMidnight(t, zone)

		today := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "срок сегодня в " + zone, "column_id": col,
			"due_date": midnight.Format(time.RFC3339),
		}), http.StatusCreated)
		past := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "срок вчера в " + zone, "column_id": col,
			"due_date": midnight.Add(-time.Second).Format(time.RFC3339),
		}), http.StatusCreated)

		sum := c.expect(t, c.get("/workspaces/"+s.WS+"/summary?tz="+url.QueryEscape(zone)), http.StatusOK)
		if sum["due_today"] != float64(1) || sum["overdue"] != float64(1) {
			t.Errorf("tz=%s: due_today=%v overdue=%v, want 1/1\nfull: %v",
				zone, sum["due_today"], sum["overdue"], sum)
		}

		// Two tasks per zone: the next iteration asserts on its own counts.
		c.expect(t, c.del("/tasks/"+today["id"].(string)), http.StatusNoContent)
		c.expect(t, c.del("/tasks/"+past["id"].(string)), http.StatusNoContent)
	}
}

// A caller that sends no zone — or an unparsable one, which is a client bug and
// not a reason to answer with numbers that silently mean something else — is
// bucketed by the zone stored in the profile. That is the same preference the
// notification scanner already evaluates quiet hours against.
func TestWorkspaceSummaryFallsBackToStoredTimezone(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	const zone = "Etc/GMT-14"
	c.expect(t, c.put("/users/me/preferences", map[string]any{
		"language": "ru", "timezone": zone, "country": "",
		"time_format": "24h", "date_format": "dd.MM.yyyy", "week_start": 1,
		"theme": "system", "accent": "purple", "board_background": "",
	}), http.StatusOK)

	midnight := localMidnight(t, zone)
	c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "срок сегодня по профилю", "column_id": col,
		"due_date": midnight.Format(time.RFC3339),
	}), http.StatusCreated)
	c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "срок вчера по профилю", "column_id": col,
		"due_date": midnight.Add(-time.Second).Format(time.RFC3339),
	}), http.StatusCreated)

	for _, q := range []string{"", "?tz=Nowhere/Notazone"} {
		sum := c.expect(t, c.get("/workspaces/"+s.WS+"/summary"+q), http.StatusOK)
		if sum["due_today"] != float64(1) || sum["overdue"] != float64(1) {
			t.Errorf("q=%q: due_today=%v overdue=%v, want 1/1\nfull: %v",
				q, sum["due_today"], sum["overdue"], sum)
		}
	}
}

// localMidnight is the start of the current calendar day in zone.
func localMidnight(t *testing.T, zone string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatalf("LoadLocation(%s): %v", zone, err)
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}
