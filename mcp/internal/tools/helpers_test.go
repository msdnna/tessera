package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

func TestPriorityLabel(t *testing.T) {
	cases := map[int]string{4: "urgent", 3: "high", 2: "normal", 1: "low", 0: "none", -1: "none", 99: "none"}
	for in, want := range cases {
		if got := priorityLabel(in); got != want {
			t.Errorf("priorityLabel(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDueString(t *testing.T) {
	if got := dueString(nil); got != "" {
		t.Errorf("nil due = %q, want empty", got)
	}
	tm := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	if got := dueString(&tm); got != "2026-07-30T12:00:00Z" {
		t.Errorf("due = %q", got)
	}
}

func TestIsOverdue(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	if !isOverdue(model.Task{DueDate: &past}, now) {
		t.Error("past due incomplete task should be overdue")
	}
	if isOverdue(model.Task{DueDate: &future}, now) {
		t.Error("future due task is not overdue")
	}
	if isOverdue(model.Task{DueDate: &past, CompletedAt: &past}, now) {
		t.Error("completed task is never overdue")
	}
	if isOverdue(model.Task{}, now) {
		t.Error("task without due date is not overdue")
	}
}

func TestSummarize(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	num := int64(42)
	est := 3.5
	glURL := "https://gitlab.example/issues/1"
	task := model.Task{
		ID: "t1", Number: &num, Title: "Do it", Priority: 3,
		ProjectName: "Proj", DueDate: &past, Estimate: &est,
		TagIDs: []string{"tag-a", "tag-missing"}, AssigneeIDs: []string{"u1", "u2"},
		GitlabURL: &glURL,
	}
	tags := map[string]string{"tag-a": "backend"}

	s := summarize(task, "В процессе", tags, "http://x/task", now)
	if s.PriorityLabel != "high" || s.Column != "В процессе" || !s.Overdue {
		t.Fatalf("summary basics: %+v", s)
	}
	if s.EstimateHours == nil || *s.EstimateHours != 3.5 || s.AssigneeCount != 2 {
		t.Fatalf("summary numerics: %+v", s)
	}
	// Only the resolvable tag id makes it into the names list.
	if len(s.Tags) != 1 || s.Tags[0] != "backend" {
		t.Fatalf("tags = %v", s.Tags)
	}
	if s.GitlabURL != glURL || s.URL != "http://x/task" {
		t.Fatalf("urls: %+v", s)
	}
}

// ── client-backed helpers via httptest ───────────────────────────────────────

// fakeAPI serves canned JSON for the handful of endpoints the enrichment
// helpers hit, keyed by exact path.
func fakeAPI(t *testing.T, routes map[string]any) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return client.New(srv.URL+"/api", "test-token")
}

func TestBoardEnrichmentAndTaskURL(t *testing.T) {
	now := time.Now()
	num := int64(7)
	routes := map[string]any{
		"/api/boards/b1":         model.Board{ID: "b1", ProjectID: "p1", Slug: "board-slug"},
		"/api/boards/b1/columns": []model.Column{{ID: "c1", Name: "К работе"}, {ID: "c2", Name: "Готово"}},
		"/api/projects/p1":       model.Project{ID: "p1", Slug: "proj-slug"},
		"/api/projects/p1/tags":  []model.Tag{{ID: "tg1", Name: "urgent"}},
	}
	c := fakeAPI(t, routes)
	e, err := boardEnrichment(context.Background(), c, "b1")
	if err != nil {
		t.Fatalf("boardEnrichment: %v", err)
	}
	if e.columns["c1"] != "К работе" || e.tags["tg1"] != "urgent" || e.projectSlug != "proj-slug" {
		t.Fatalf("enrichment: %+v", e)
	}

	task := model.Task{ID: "t1", ColumnID: "c1", Number: &num, TagIDs: []string{"tg1"}}
	s := e.summarize(task, now)
	if s.Column != "К работе" || len(s.Tags) != 1 {
		t.Fatalf("enriched summary: %+v", s)
	}
	wantURL := c.WebBaseURL() + "/project/proj-slug/board/board-slug?task=7"
	if s.URL != wantURL {
		t.Fatalf("task url = %q, want %q", s.URL, wantURL)
	}

	// Missing number → no deep link.
	if url := e.taskURL(model.Task{ColumnID: "c1"}); url != "" {
		t.Fatalf("task url without number = %q, want empty", url)
	}
}

func TestColumnNameAndWorkspaceTags(t *testing.T) {
	routes := map[string]any{
		"/api/boards/b1/columns":  []model.Column{{ID: "c1", Name: "One"}, {ID: "c2", Name: "Two"}},
		"/api/workspaces/w1/tags": []model.Tag{{ID: "t1", Name: "alpha"}, {ID: "t2", Name: "beta"}},
	}
	c := fakeAPI(t, routes)
	if got := columnName(context.Background(), c, "b1", "c2"); got != "Two" {
		t.Fatalf("columnName = %q", got)
	}
	if got := columnName(context.Background(), c, "b1", "missing"); got != "" {
		t.Fatalf("columnName(missing) = %q, want empty", got)
	}
	m, err := workspaceTagNames(context.Background(), c, "w1")
	if err != nil || m["t1"] != "alpha" || m["t2"] != "beta" {
		t.Fatalf("workspaceTagNames: %v %v", m, err)
	}
}

func TestBlockedTaskIDs(t *testing.T) {
	// t2 blocked_by t1 (open) → blocked; t3 blocks t4 but t3 is done → t4 free.
	done := time.Now()
	routes := map[string]any{
		"/api/boards/b1/dependencies": []model.Dependency{
			{Kind: "blocked_by", TaskID: "t2", RelatedTaskID: "t1"},
			{Kind: "blocks", TaskID: "t3", RelatedTaskID: "t4"},
			{Kind: "relates", TaskID: "t5", RelatedTaskID: "t6"},
		},
	}
	c := fakeAPI(t, routes)
	tasks := []model.Task{{ID: "t1"}, {ID: "t2"}, {ID: "t3", CompletedAt: &done}, {ID: "t4"}}
	blocked, err := blockedTaskIDs(context.Background(), c, "b1", tasks)
	if err != nil {
		t.Fatalf("blockedTaskIDs: %v", err)
	}
	if !blocked["t2"] {
		t.Error("t2 should be blocked by open t1")
	}
	if blocked["t4"] {
		t.Error("t4 should be free (its blocker t3 is done)")
	}
	if blocked["t5"] || blocked["t6"] {
		t.Error("relates edges must not gate work")
	}
}
