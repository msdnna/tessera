package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The second phase of a full sweep (#3105) fetches discussions only for issues that
// are new or whose updatedAt moved since the link's last sync.
func TestNeedNotes(t *testing.T) {
	t0 := time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Minute)
	issues := []Issue{
		{GlobalID: "g1", IID: 1, UpdatedAt: &t0},                    // linked, unchanged
		{GlobalID: "g2", IID: 2, UpdatedAt: &t1},                    // linked, changed
		{GlobalID: "g3", IID: 3, UpdatedAt: &t0},                    // new
		{GlobalID: "g4", IID: 4, UpdatedAt: &t0, State: "closed"},   // new, skipped by policy
		{GlobalID: "g5", IID: 5, UpdatedAt: nil},                    // linked, GitLab sent no time
		{GlobalID: "g6", IID: 6, UpdatedAt: &t1, NotesLoaded: true}, // already has its notes
	}
	known := map[string]SyncKey{
		"g1": {UpdatedAt: &t0}, "g2": {UpdatedAt: &t0}, "g5": {UpdatedAt: &t0}, "g6": {UpdatedAt: &t0},
	}
	got := NeedNotes(issues, known, func(is Issue) bool { return is.State == "closed" })
	if fmt.Sprint(got) != "[2 3 5]" {
		t.Fatalf("NeedNotes = %v, want [2 3 5]", got)
	}
}

func TestWithNotes(t *testing.T) {
	light := []Issue{{GlobalID: "a", Title: "A"}, {GlobalID: "b", Title: "B"}}
	full := []Issue{{GlobalID: "b", Title: "B", NotesLoaded: true, Notes: []Note{{Body: "x"}}}}
	got := WithNotes(light, full)
	if len(got) != 2 || got[0].NotesLoaded || !got[1].NotesLoaded || len(got[1].Notes) != 1 {
		t.Fatalf("WithNotes = %+v", got)
	}
}

// The light query of a full sweep (#3105) leaves the discussions out, starts at the
// largest page and keeps its own page-size memory; the label filter (#3085) rides
// in the same request, and only there.
func TestListIssues_LightQueryAndLabels(t *testing.T) {
	var queries []string
	var vars []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		queries = append(queries, req.Query)
		vars = append(vars, req.Variables)
		_, _ = fmt.Fprint(w, `{"data":{"project":{"issues":{"pageInfo":{"hasNextPage":false,"endCursor":""},
			"nodes":[{"id":"gid://gitlab/Issue/1","iid":"1","title":"one"}]}}}}`)
	}))
	t.Cleanup(srv.Close)

	issues, err := New(srv.URL, "tok").ListIssues(context.Background(), "grp/light",
		IssueFilter{Light: true, Labels: []string{" backend ", "", "backend", "api"}})
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 || issues[0].NotesLoaded {
		t.Fatalf("issues = %+v, want one light issue", issues)
	}
	if strings.Contains(queries[0], "discussions") {
		t.Fatal("the light query asked for discussions")
	}
	if first := int(vars[0]["first"].(float64)); first != lightIssuesPageSize {
		t.Fatalf("light first = %d, want %d", first, lightIssuesPageSize)
	}
	if got := fmt.Sprint(vars[0]["labels"]); got != "[backend api]" {
		t.Fatalf("labels var = %s, want [backend api]", got)
	}

	full, err := New(srv.URL, "tok").IssuesByIIDs(context.Background(), "grp/light", []string{"1"})
	if err != nil {
		t.Fatalf("IssuesByIIDs: %v", err)
	}
	if !strings.Contains(queries[1], "discussions") || !full[0].NotesLoaded {
		t.Fatal("IssuesByIIDs must carry the discussions")
	}
	if _, ok := vars[1]["labels"]; ok {
		t.Fatal("IssuesByIIDs must not filter by label")
	}
}

// A long iid list is asked for in chunks, so the variables of one request stay
// bounded on a sweep that names a few thousand issues.
func TestIssuesByIIDs_Chunks(t *testing.T) {
	var sizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		sizes = append(sizes, len(req.Variables["iids"].([]any)))
		_, _ = fmt.Fprint(w, `{"data":{"project":{"issues":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}`)
	}))
	t.Cleanup(srv.Close)

	iids := make([]string, 250)
	for i := range iids {
		iids[i] = fmt.Sprint(i + 1)
	}
	if _, err := New(srv.URL, "tok").IssuesByIIDsLight(context.Background(), "grp/chunk", iids); err != nil {
		t.Fatalf("IssuesByIIDsLight: %v", err)
	}
	if fmt.Sprint(sizes) != "[100 100 50]" {
		t.Fatalf("chunk sizes = %v, want [100 100 50]", sizes)
	}
}
