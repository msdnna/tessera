package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tessera-mcp/internal/client"
	"tessera-mcp/internal/model"
)

// routeAPI serves canned JSON keyed by exact path (bare arrays / objects, as
// the REST client decodes them). Unknown paths → 404.
func routeAPI(t *testing.T, routes map[string]any) *client.Client {
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
	return client.New(srv.URL+"/api", "tok")
}

func TestListWorkspacesHandler(t *testing.T) {
	c := routeAPI(t, map[string]any{
		"/api/workspaces": []model.Workspace{{ID: "w1", Name: "Personal"}, {ID: "w2", Name: "Team"}},
	})
	_, out, err := listWorkspaces(c)(context.Background(), nil, struct{}{})
	if err != nil || len(out.Workspaces) != 2 || out.Workspaces[0].Name != "Personal" {
		t.Fatalf("listWorkspaces: %+v %v", out, err)
	}
}

func TestListProjectsHandler(t *testing.T) {
	c := routeAPI(t, map[string]any{
		"/api/workspaces/w1/projects": []model.Project{{ID: "p1", Name: "Proj", Slug: "proj"}},
	})
	// Missing workspace_id → error, no request.
	if _, _, err := listProjects(c)(context.Background(), nil, workspaceInput{}); err == nil {
		t.Fatal("expected error for empty workspace_id")
	}
	_, out, err := listProjects(c)(context.Background(), nil, workspaceInput{WorkspaceID: "w1"})
	if err != nil || len(out.Projects) != 1 || out.Projects[0].Slug != "proj" {
		t.Fatalf("listProjects: %+v %v", out, err)
	}
}

func TestListBoardsHandler(t *testing.T) {
	c := routeAPI(t, map[string]any{
		"/api/projects/p1/boards": []model.Board{{ID: "b1", ProjectID: "p1", Name: "Board", Slug: "board"}},
	})
	if _, _, err := listBoards(c)(context.Background(), nil, projectInput{}); err == nil {
		t.Fatal("expected error for empty project_id")
	}
	_, out, err := listBoards(c)(context.Background(), nil, projectInput{ProjectID: "p1"})
	if err != nil || len(out.Boards) != 1 || out.Boards[0].Slug != "board" {
		t.Fatalf("listBoards: %+v %v", out, err)
	}
}

func TestResolveBoardHandler(t *testing.T) {
	c := routeAPI(t, map[string]any{
		"/api/board-by-slug": model.Board{ID: "b1", ProjectID: "p1", Name: "Board", Slug: "board"},
	})
	if _, _, err := resolveBoard(c)(context.Background(), nil, resolveBoardInput{ProjectSlug: "p"}); err == nil {
		t.Fatal("expected error when a slug is missing")
	}
	_, out, err := resolveBoard(c)(context.Background(), nil,
		resolveBoardInput{ProjectSlug: "proj", BoardSlug: "board"})
	if err != nil || out.ID != "b1" {
		t.Fatalf("resolveBoard: %+v %v", out, err)
	}
}

// boardRoutes returns the enrichment endpoints a board listing needs, plus the
// task list at the given path.
func boardRoutes(tasks []model.Task) map[string]any {
	return map[string]any{
		"/api/boards/b1":         model.Board{ID: "b1", ProjectID: "p1", Slug: "board"},
		"/api/boards/b1/columns": []model.Column{{ID: "c1", Name: "К работе"}, {ID: "c2", Name: "Готово"}},
		"/api/projects/p1":       model.Project{ID: "p1", Slug: "proj"},
		"/api/projects/p1/tags":  []model.Tag{{ID: "tg1", Name: "backend"}},
		"/api/boards/b1/tasks":   tasks,
	}
}

func TestListTasksHandler(t *testing.T) {
	num := int64(3)
	tasks := []model.Task{
		{ID: "t1", ColumnID: "c1", Title: "Open", Number: &num, TagIDs: []string{"tg1"}},
	}
	c := routeAPI(t, boardRoutes(tasks))
	if _, _, err := listTasks(c)(context.Background(), nil, listTasksInput{}); err == nil {
		t.Fatal("expected error for empty board_id")
	}
	_, out, err := listTasks(c)(context.Background(), nil, listTasksInput{BoardID: "b1"})
	if err != nil || out.Count != 1 || out.Tasks[0].Column != "К работе" || len(out.Tasks[0].Tags) != 1 {
		t.Fatalf("listTasks: %+v %v", out, err)
	}
}

func TestMyTasksHandler(t *testing.T) {
	c := routeAPI(t, map[string]any{
		"/api/workspaces/w1/tags": []model.Tag{{ID: "tg1", Name: "urgent"}},
		"/api/workspaces/w1/tasks": []model.Task{
			{ID: "t1", Title: "Mine", ColumnName: "Doing", TagIDs: []string{"tg1"}},
		},
	})
	if _, _, err := myTasks(c)(context.Background(), nil, workspaceInput{}); err == nil {
		t.Fatal("expected error for empty workspace_id")
	}
	_, out, err := myTasks(c)(context.Background(), nil, workspaceInput{WorkspaceID: "w1"})
	if err != nil || out.Count != 1 || out.Tasks[0].Column != "Doing" {
		t.Fatalf("myTasks: %+v %v", out, err)
	}
}

func TestNextTaskHandler(t *testing.T) {
	// Board mode: t1 blocked_by open t2 → next actionable is t2.
	num1, num2 := int64(1), int64(2)
	routes := boardRoutes([]model.Task{
		{ID: "t1", ColumnID: "c1", Title: "Blocked", Number: &num1},
		{ID: "t2", ColumnID: "c1", Title: "Free", Number: &num2},
	})
	routes["/api/boards/b1/dependencies"] = []model.Dependency{
		{Kind: "blocked_by", TaskID: "t1", RelatedTaskID: "t2"},
	}
	c := routeAPI(t, routes)
	_, out, err := nextTask(c)(context.Background(), nil, nextTaskInput{BoardID: "b1"})
	if err != nil || !out.Found || out.Task == nil || out.Task.Title != "Free" {
		t.Fatalf("nextTask board: %+v %v", out, err)
	}

	// Neither id → error.
	if _, _, err := nextTask(c)(context.Background(), nil, nextTaskInput{}); err == nil {
		t.Fatal("expected error when neither board_id nor workspace_id given")
	}

	// Workspace mode with no tasks → Found=false + note.
	wc := routeAPI(t, map[string]any{
		"/api/workspaces/w9/tags":  []model.Tag{},
		"/api/workspaces/w9/tasks": []model.Task{},
	})
	_, out, err = nextTask(wc)(context.Background(), nil, nextTaskInput{WorkspaceID: "w9"})
	if err != nil || out.Found || out.Note == "" {
		t.Fatalf("nextTask empty workspace: %+v %v", out, err)
	}
}

func TestGetTaskHandlerValidation(t *testing.T) {
	c := routeAPI(t, map[string]any{})
	// Neither task_id nor workspace_id+number → error before any request.
	if _, _, err := getTask(c)(context.Background(), nil, getTaskInput{}); err == nil {
		t.Fatal("expected error for missing identifiers")
	}
}
