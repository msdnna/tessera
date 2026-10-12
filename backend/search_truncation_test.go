// Global search hands back at most a page of each kind, and has to say so.
//
// The three queries behind /workspaces/:id/search carry a LIMIT, which is the
// right call — the endpoint fires from a debounced input on every keystroke and
// nobody reads a thousand rows in a dropdown. What was wrong is that the cap was
// invisible: a term that matches half the tracker produced exactly the same
// response shape as one that matches four things, so the reader saw an arbitrary
// slice ordered by number and no sign that the thing they were looking for had
// been cut off the end. The honest answer names the cap.
package main

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// A search that fills the page says there is more; one that does not, does not.
//
// Both halves matter. The flag existing is useless if it is always true — the
// hint would sit under every search anyone ever ran and be worth nothing — so
// the under-cap case is asserted in the same test, against the same workspace,
// with a term that matches a known handful.
func TestSearchReportsTruncation(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	// One past the page: the boundary is where an off-by-one lives, and a
	// probe that fetched exactly the cap could never tell "full page" from
	// "full page plus more".
	const over = 26
	for i := 0; i < over; i++ {
		mkTask(t, c, s.Board, col, fmt.Sprintf("Перебор строк %02d", i))
	}
	// Distinct term, matched by three of the tasks above and nothing else.
	for i := 0; i < 3; i++ {
		mkTask(t, c, s.Board, col, fmt.Sprintf("Одинокая находка %d", i))
	}

	full := c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+url.QueryEscape("Перебор")), http.StatusOK)
	tasks, _ := full["tasks"].([]any)
	if len(tasks) != 25 {
		t.Fatalf("tasks returned = %d, want the page size 25", len(tasks))
	}
	if !hasMore(t, full, "tasks") {
		t.Fatalf("a search that filled the page reports no truncation: %v", full["has_more"])
	}
	// The page is a page, not a page plus the probe row that measured it.
	if hasMore(t, full, "notes") || hasMore(t, full, "documents") {
		t.Fatalf("kinds with no matches at all report truncation: %v", full["has_more"])
	}

	few := c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+url.QueryEscape("Одинокая")), http.StatusOK)
	if got, _ := few["tasks"].([]any); len(got) != 3 {
		t.Fatalf("tasks returned = %d, want 3", len(got))
	}
	if hasMore(t, few, "tasks") {
		t.Fatalf("a search well under the cap claims to be truncated: %v", few["has_more"])
	}
}

// hasMore reads one kind's truncation flag, failing the test if the response
// does not carry the map at all — an absent flag reads as false everywhere in
// JavaScript, which is exactly how this would regress unnoticed.
func hasMore(t *testing.T, body map[string]any, kind string) bool {
	t.Helper()
	more, ok := body["has_more"].(map[string]any)
	if !ok {
		t.Fatalf("response carries no has_more map: %v", body)
	}
	v, ok := more[kind].(bool)
	if !ok {
		t.Fatalf("has_more has no %q flag: %v", kind, more)
	}
	return v
}
