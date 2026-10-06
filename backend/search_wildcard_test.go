// Workspace search builds its ILIKE pattern by concatenation —
// `'%' || $2 || '%'` (db/queries/search.sql) — so the characters LIKE reserves
// for itself arrive inside the pattern as operators rather than as text. `_`
// stands for "any one character" and `%` for "any run of characters", and a
// tracker whose own subject matter is code is exactly where users type them:
// `created_at`, `task_assignees`, `gitlab_webhook_cron`.
//
// The damage is quiet — the search answers, it just answers about a different
// query than the one that was typed. `perf_report` silently also matches
// `perf-report` and `perfXreport`; a `%` anywhere collapses the query into
// "return the first 25 by number", which reads as a plausible result list and
// not as a failure.
//
// The fix escapes the three characters LIKE reserves (`\`, `%`, `_`) in the
// handler rather than in SQL: the query text stays byte-for-byte what the
// trigram indexes of migration 0052 were built for, so the plan does not change.
package main

import (
	"net/http"
	"net/url"
	"testing"
)

// searchTitles pulls the titles out of one bucket of a search response.
func searchTitles(t *testing.T, res map[string]any, bucket string) []string {
	t.Helper()
	rows, _ := res[bucket].([]any)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		m, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("%s: unexpected row %#v", bucket, r)
		}
		title, _ := m["title"].(string)
		out = append(out, title)
	}
	return out
}

// TestSearchTreatsWildcardsAsText pins that a query means itself: the LIKE
// metacharacters a user types are matched literally, in all three buckets.
func TestSearchTreatsWildcardsAsText(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	mk := s.WS[:8]

	// `_` — the shape a code identifier has. The decoy differs from the wanted
	// row in exactly the one character the wildcard would wave through.
	wantTask := "Отчёт perf_report" + mk
	mkTask(t, c, s.Board, s.col(t, 0), wantTask)
	mkTask(t, c, s.Board, s.col(t, 0), "Отчёт perfXreport"+mk)

	res := c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+url.QueryEscape("perf_report"+mk)), http.StatusOK)
	if got := searchTitles(t, res, "tasks"); len(got) != 1 || got[0] != wantTask {
		t.Fatalf("`_` in a task query matched as a wildcard: %v", got)
	}

	// `%` — the shape a percentage has. Without escaping the pattern becomes
	// `%50%<mk>%`, which the "5000 рублей" decoy satisfies too.
	wantNote := "Скидка 50%" + mk
	c.expect(t, c.post("/workspaces/"+s.WS+"/notes",
		map[string]any{"title": wantNote}), http.StatusCreated)
	c.expect(t, c.post("/workspaces/"+s.WS+"/notes",
		map[string]any{"title": "Скидка 5000 рублей " + mk}), http.StatusCreated)

	res = c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+url.QueryEscape("50%"+mk)), http.StatusOK)
	if got := searchTitles(t, res, "notes"); len(got) != 1 || got[0] != wantNote {
		t.Fatalf("`%%` in a note query matched as a wildcard: %v", got)
	}

	// Documents match on the title only — same rule, third bucket.
	wantDoc := "Схема api_v2" + mk
	c.expect(t, c.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": wantDoc}), http.StatusCreated)
	c.expect(t, c.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "Схема apiXv2" + mk}), http.StatusCreated)

	res = c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+url.QueryEscape("api_v2"+mk)), http.StatusOK)
	if got := searchTitles(t, res, "documents"); len(got) != 1 || got[0] != wantDoc {
		t.Fatalf("`_` in a document query matched as a wildcard: %v", got)
	}
}

// TestSearchEscapeCharIsText covers the escape character itself: a backslash is
// what LIKE uses to quote the other two, so an unescaped one either swallows the
// character after it or — as a trailing byte — makes Postgres reject the pattern
// outright, turning a search into a 500.
func TestSearchEscapeCharIsText(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	mk := s.WS[:8]

	wantTask := `Путь C:\build` + mk
	mkTask(t, c, s.Board, s.col(t, 0), wantTask)
	mkTask(t, c, s.Board, s.col(t, 0), "Путь C:build"+mk)

	res := c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+url.QueryEscape(`C:\build`+mk)), http.StatusOK)
	if got := searchTitles(t, res, "tasks"); len(got) != 1 || got[0] != wantTask {
		t.Fatalf(`\ in a task query was read as an escape: %v`, got)
	}

	// A query that ends on the escape character must still be a search, not an
	// error: the pattern `%…\%` leaves LIKE with a dangling escape.
	c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+url.QueryEscape(`незавершённый`+mk+`\`)), http.StatusOK)
}
