package tools

import (
	"context"
	"encoding/json"
	"testing"

	"tessera-mcp/internal/model"
)

func TestCreateTaskResolvesColumnAndTags(t *testing.T) {
	c, mux := newMux(t, map[string]any{
		"/api/boards/b1": model.Board{ID: "b1", ProjectID: "p1"},
		"/api/boards/b1/columns": []model.Column{
			{ID: "c2", Name: "В процессе", Position: 2}, {ID: "c1", Name: "К работе", Position: 1},
		},
		"/api/projects/p1":      model.Project{ID: "p1", WorkspaceID: "w1", Slug: "tessera"},
		"/api/projects/p1/tags": []model.Tag{{ID: "tag-bug", ProjectID: "p1", Name: "type::bug"}},
	})
	ctx := context.Background()

	if _, _, err := createTask(c)(ctx, nil, createTaskInput{BoardID: "b1"}); err == nil {
		t.Fatal("expected error for missing title")
	}
	if _, _, err := createTask(c)(ctx, nil,
		createTaskInput{taskFieldsInput: taskFieldsInput{Title: "T"}}); err == nil {
		t.Fatal("expected error without board or parent")
	}

	// An unknown tag fails before anything is created (create_missing is off).
	if _, _, err := createTask(c)(ctx, nil, createTaskInput{
		BoardID: "b1", taskFieldsInput: taskFieldsInput{Title: "T", Tags: []string{"nope"}},
	}); err == nil {
		t.Fatal("expected error for unknown tag")
	}
	if _, ok := mux.writes["POST /api/boards/b1/tasks"]; ok {
		t.Fatal("task created despite the tag failing to resolve")
	}

	// No column given → leftmost by position, not first in the response.
	_, out, err := createTask(c)(ctx, nil, createTaskInput{
		BoardID: "b1",
		taskFieldsInput: taskFieldsInput{
			Title: "Fix it", Priority: 3, DueDate: "2026-08-21", Tags: []string{"type::bug"},
		},
	})
	if err != nil {
		t.Fatalf("createTask: %v", err)
	}
	if out.Column != "К работе" {
		t.Fatalf("column = %q, want the leftmost", out.Column)
	}
	var body map[string]any
	_ = json.Unmarshal(mux.writes["POST /api/boards/b1/tasks"], &body)
	if body["column_id"] != "c1" || body["title"] != "Fix it" || body["priority"] != 3.0 {
		t.Fatalf("create body = %v", body)
	}
	if _, ok := body["parent_id"]; ok {
		t.Fatalf("parent_id sent for a top-level task: %v", body)
	}
	if _, ok := mux.writes["POST /api/tasks/cm-new/tags"]; !ok {
		t.Fatalf("tag not attached: %v", mux.writes)
	}
}

func TestCreateTaskRejectsBadInput(t *testing.T) {
	c, _ := newMux(t, map[string]any{
		"/api/boards/b1":         model.Board{ID: "b1", ProjectID: "p1"},
		"/api/boards/b1/columns": []model.Column{{ID: "c1", Name: "К работе", Position: 1}},
	})
	ctx := context.Background()
	for name, in := range map[string]createTaskInput{
		"bad priority": {BoardID: "b1", taskFieldsInput: taskFieldsInput{Title: "T", Priority: 9}},
		"bad date":     {BoardID: "b1", taskFieldsInput: taskFieldsInput{Title: "T", DueDate: "завтра"}},
		"bad column":   {BoardID: "b1", Column: "Nope", taskFieldsInput: taskFieldsInput{Title: "T"}},
	} {
		if _, _, err := createTask(c)(ctx, nil, in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestCreateSubtasksReportsPartialFailure(t *testing.T) {
	parent := detail("t1", "b1")
	parent.ColumnID = "c1"
	c, _ := newMux(t, map[string]any{
		"/api/tasks/t1":          parent,
		"/api/boards/b1":         model.Board{ID: "b1", ProjectID: "p1"},
		"/api/boards/b1/columns": []model.Column{{ID: "c1", Name: "К работе", Position: 1}},
		"/api/projects/p1":       model.Project{ID: "p1", WorkspaceID: "w1"},
		"/api/projects/p1/tags":  []model.Tag{},
	})
	ctx := context.Background()

	if _, _, err := createSubtasks(c)(ctx, nil, createSubtasksInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error for empty items")
	}
	// One good item, one with an unresolvable tag: the good one still lands.
	_, out, err := createSubtasks(c)(ctx, nil, createSubtasksInput{
		TaskID: "t1",
		Items: []taskFieldsInput{
			{Title: "Step 1"},
			{Title: "Step 2", Tags: []string{"ghost"}},
		},
	})
	if err != nil {
		t.Fatalf("createSubtasks: %v", err)
	}
	if len(out.Created) != 1 || out.Created[0].Title != "New" {
		t.Fatalf("created = %+v", out.Created)
	}
	if len(out.Failed) != 1 || out.Failed[0].Title != "Step 2" {
		t.Fatalf("failed = %+v", out.Failed)
	}
}

func TestUpdateTaskSendsOnlyGivenFields(t *testing.T) {
	c, mux := newMux(t, map[string]any{"/api/tasks/t1": detail("t1", "b1")})
	ctx := context.Background()

	if _, _, err := updateTask(c)(ctx, nil, updateTaskInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error when nothing is being updated")
	}
	empty, zero, bad := "  ", 0.0, 9
	for name, in := range map[string]updateTaskInput{
		"empty title":   {TaskID: "t1", Title: &empty},
		"zero estimate": {TaskID: "t1", EstimateHours: &zero},
		"bad priority":  {TaskID: "t1", Priority: &bad},
		"unknown clear": {TaskID: "t1", Clear: []string{"title"}},
	} {
		if _, _, err := updateTask(c)(ctx, nil, in); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}

	title := "Renamed"
	_, out, err := updateTask(c)(ctx, nil, updateTaskInput{
		TaskID: "t1", Title: &title, Clear: []string{"estimate_hours"},
	})
	if err != nil || len(out.Changed) != 2 {
		t.Fatalf("updateTask: %+v %v", out, err)
	}
	var body map[string]json.RawMessage
	_ = json.Unmarshal(mux.writes["PATCH /api/tasks/t1"], &body)
	if len(body) != 2 || string(body["title"]) != `"Renamed"` || string(body["estimate"]) != "null" {
		t.Fatalf("patch body = %s", mux.writes["PATCH /api/tasks/t1"])
	}
}

func TestSetParentAttachAndDetach(t *testing.T) {
	c, mux := newMux(t, map[string]any{
		"/api/tasks/t1": detail("t1", "b1"),
		"/api/tasks/t2": detail("t2", "b1"),
	})
	ctx := context.Background()

	// Neither parent nor detach, and both at once, are both refused.
	if _, _, err := setParent(c)(ctx, nil, setParentInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error without parent or detach")
	}
	if _, _, err := setParent(c)(ctx, nil, setParentInput{TaskID: "t1", ParentID: "t2", Detach: true}); err == nil {
		t.Fatal("expected error for parent + detach")
	}

	if _, out, err := setParent(c)(ctx, nil, setParentInput{TaskID: "t1", ParentID: "t2"}); err != nil || out.ParentID != "t2" {
		t.Fatalf("attach: %+v %v", out, err)
	}
	if body := string(mux.writes["PATCH /api/tasks/t1/parent"]); body != `{"parent_id":"t2"}` {
		t.Fatalf("attach body = %s", body)
	}
	if _, out, err := setParent(c)(ctx, nil, setParentInput{TaskID: "t1", Detach: true}); err != nil || !out.Detached {
		t.Fatalf("detach: %+v %v", out, err)
	}
	if body := string(mux.writes["PATCH /api/tasks/t1/parent"]); body != `{"parent_id":null}` {
		t.Fatalf("detach body = %s", body)
	}
}

func TestMoveDescriptionWritesTargetBeforeClearingSource(t *testing.T) {
	src := detail("t1", "b1")
	src.Description = "Спека фичи"
	dst := detail("t2", "b1")
	dst.Description = "Уже есть"
	c, mux := newMux(t, map[string]any{"/api/tasks/t1": src, "/api/tasks/t2": dst})
	ctx := context.Background()

	if _, _, err := moveDescription(c)(ctx, nil,
		moveDescriptionInput{FromTaskID: "t1", ToTaskID: "t1"}); err == nil {
		t.Fatal("expected error when source and target are the same")
	}
	if _, _, err := moveDescription(c)(ctx, nil,
		moveDescriptionInput{FromTaskID: "t2", ToTaskID: "t1", Mode: "bogus"}); err == nil {
		t.Fatal("expected error for bad mode")
	}

	_, out, err := moveDescription(c)(ctx, nil, moveDescriptionInput{
		FromTaskID: "t1", ToTaskID: "t2", Heading: "Из #1", Cut: true,
	})
	if err != nil || !out.SourceEmptied {
		t.Fatalf("moveDescription: %+v %v", out, err)
	}
	var target, source map[string]any
	_ = json.Unmarshal(mux.writes["PATCH /api/tasks/t2"], &target)
	_ = json.Unmarshal(mux.writes["PATCH /api/tasks/t1"], &source)
	desc, _ := target["description"].(string)
	if want := "Уже есть\n\n---\n\n## Из #1\n\nСпека фичи"; desc != want {
		t.Fatalf("target description = %q, want %q", desc, want)
	}
	if source["description"] != "" {
		t.Fatalf("source not emptied: %v", source)
	}
}

func TestParseDate(t *testing.T) {
	if got, err := parseDate(""); err != nil || got != nil {
		t.Fatalf("empty date = %v %v", got, err)
	}
	got, err := parseDate("2026-08-21")
	if err != nil || got.Year() != 2026 || got.Month() != 8 || got.Day() != 21 {
		t.Fatalf("iso date = %v %v", got, err)
	}
	if _, err := parseDate("2026-08-21T10:30:00Z"); err != nil {
		t.Fatalf("rfc3339: %v", err)
	}
	// Relative wording belongs to the backend's quick actions, not here.
	if _, err := parseDate("завтра"); err == nil {
		t.Fatal("expected an error for relative wording")
	}
}
