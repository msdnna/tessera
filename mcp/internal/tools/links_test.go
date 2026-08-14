package tools

import (
	"context"
	"encoding/json"
	"testing"

	"tessera-mcp/internal/model"
)

// tagBoard is the canned board/project/tag context the tag tests share:
// task t1 on board b1 in project p1, which owns the given tags.
func tagBoard(existing, taskTags []model.Tag) map[string]any {
	d := detail("t1", "b1")
	d.Tags = taskTags
	return map[string]any{
		"/api/tasks/t1":         d,
		"/api/boards/b1":        model.Board{ID: "b1", ProjectID: "p1"},
		"/api/projects/p1/tags": existing,
	}
}

func TestSetTagsAddAndRemove(t *testing.T) {
	c, mux := newMux(t, tagBoard(
		[]model.Tag{{ID: "tag-bug", Name: "type::bug"}, {ID: "tag-fe", Name: "app::frontend"}},
		[]model.Tag{{ID: "tag-bug", Name: "type::bug"}}))
	ctx := context.Background()

	if _, _, err := setTags(c)(ctx, nil, setTagsInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error when neither add nor remove is given")
	}
	// An unknown name is an error rather than a silently created tag.
	if _, _, err := setTags(c)(ctx, nil, setTagsInput{TaskID: "t1", Add: []string{"typo::bg"}}); err == nil {
		t.Fatal("expected error for unknown tag without create_missing")
	}
	if _, ok := mux.writes["POST /api/projects/p1/tags"]; ok {
		t.Fatal("tag created without create_missing")
	}

	_, out, err := setTags(c)(ctx, nil, setTagsInput{
		TaskID: "t1", Add: []string{"app::frontend"}, Remove: []string{"type::bug"},
	})
	if err != nil {
		t.Fatalf("setTags: %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "type::bug" { // echoes the re-read detail
		t.Fatalf("tags = %v", out.Tags)
	}
	var body map[string]any
	_ = json.Unmarshal(mux.writes["POST /api/tasks/t1/tags"], &body)
	if body["tag_id"] != "tag-fe" {
		t.Fatalf("add body = %v", body)
	}
	if _, ok := mux.writes["DELETE /api/tasks/t1/tags/tag-bug"]; !ok {
		t.Fatalf("tag not removed: %v", mux.writes)
	}
}

func TestSetTagsCreateMissing(t *testing.T) {
	c, mux := newMux(t, tagBoard([]model.Tag{}, nil))
	if _, _, err := setTags(c)(context.Background(), nil,
		setTagsInput{TaskID: "t1", Add: []string{"новый"}, CreateMissing: true}); err != nil {
		t.Fatalf("setTags: %v", err)
	}
	var created map[string]any
	_ = json.Unmarshal(mux.writes["POST /api/projects/p1/tags"], &created)
	if created["name"] != "новый" {
		t.Fatalf("created tag body = %v", created)
	}
	// The freshly created tag id (cm-new from the canned write response) is attached.
	var attached map[string]any
	_ = json.Unmarshal(mux.writes["POST /api/tasks/t1/tags"], &attached)
	if attached["tag_id"] != "cm-new" {
		t.Fatalf("attach body = %v", attached)
	}
}

func TestLinkTasksValidatesKind(t *testing.T) {
	num := int64(10)
	d := detail("t1", "b1")
	d.Number = &num
	c, mux := newMux(t, map[string]any{"/api/tasks/t1": d})
	ctx := context.Background()

	if _, _, err := linkTasks(c)(ctx, nil, linkTasksInput{TaskID: "t1"}); err == nil {
		t.Fatal("expected error for missing related_number")
	}
	if _, _, err := linkTasks(c)(ctx, nil,
		linkTasksInput{TaskID: "t1", RelatedNumber: 11, Kind: "sorta-relates"}); err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if _, _, err := linkTasks(c)(ctx, nil,
		linkTasksInput{TaskID: "t1", RelatedNumber: 10}); err == nil {
		t.Fatal("expected error when linking a task to itself")
	}

	_, out, err := linkTasks(c)(ctx, nil, linkTasksInput{TaskID: "t1", RelatedNumber: 11})
	if err != nil || out.Kind != "relates" { // kind defaults to relates
		t.Fatalf("linkTasks: %+v %v", out, err)
	}
	var body map[string]any
	_ = json.Unmarshal(mux.writes["POST /api/tasks/t1/relations"], &body)
	if body["number"] != 11.0 || body["kind"] != "relates" {
		t.Fatalf("relation body = %v", body)
	}
}

func TestUnlinkTasksClearsBothDirections(t *testing.T) {
	c, mux := newMux(t, map[string]any{
		"/api/tasks/t1":                         detail("t1", "b1"),
		"/api/workspaces/w1/tasks/by-number/11": detail("t2", "b1"),
		// The link was recorded on t1; t2 additionally carries a stale reverse row.
		"/api/tasks/t1/relations": []model.Relation{
			{ID: "r1", TaskID: "t1", RelatedTaskID: "t2", Kind: "blocks"},
			{ID: "r2", TaskID: "t1", RelatedTaskID: "t9", Kind: "relates"},
		},
		"/api/tasks/t2/relations": []model.Relation{
			{ID: "r3", TaskID: "t2", RelatedTaskID: "t1", Kind: "blocked_by"},
		},
	})
	ctx := context.Background()

	if _, _, err := unlinkTasks(c)(ctx, nil, unlinkTasksInput{TaskID: "t1", WorkspaceID: "w1"}); err == nil {
		t.Fatal("expected error for missing related_number")
	}
	_, out, err := unlinkTasks(c)(ctx, nil,
		unlinkTasksInput{TaskID: "t1", WorkspaceID: "w1", RelatedNumber: 11})
	if err != nil || out.Removed != 2 {
		t.Fatalf("unlinkTasks: %+v %v", out, err)
	}
	for _, want := range []string{"DELETE /api/relations/r1", "DELETE /api/relations/r3"} {
		if _, ok := mux.writes[want]; !ok {
			t.Fatalf("%s not sent: %v", want, mux.writes)
		}
	}
	// The unrelated link on the same task is left alone.
	if _, ok := mux.writes["DELETE /api/relations/r2"]; ok {
		t.Fatal("unrelated relation deleted")
	}
}
