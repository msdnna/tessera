package main

import (
	"net/http"
	"testing"
)

// TestTransferProjectStripsCrossWorkspaceTags pins the fifth reference a project
// transfer used to leave hanging across the workspace boundary it creates
// (#2932): the tag on a task.
//
// applyTag deliberately lets a tag cross *projects* inside one workspace ("which
// the board views rely on") and refuses one from another workspace, spelling out
// why: a stranger's tag on the caller's task means "the task detail reads its
// name and colour back, a cross-workspace read of data the caller was never
// shown". A transfer moves the boundary under a pairing that was legal when it
// was made, and task_tags was never re-checked:
//
//   - a task that travels keeps a tag of a project that stayed behind, so the
//     target workspace reads the source's tag name and colour;
//   - a task that stays keeps a tag of the project that left — that tag's
//     workspace_id was just re-stamped to the target — so the source workspace
//     reads the target's.
//
// ListTaskTags joins tags straight through task_tags with no workspace filter of
// its own, so both directions surface in GET /tasks/:id.
func TestTransferProjectStripsCrossWorkspaceTags(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	src := mkStack(t, owner)
	dst := mkStack(t, owner)

	// A second project in the source workspace: it stays behind when the first
	// one leaves, and that is what puts a tag and its task apart.
	stayProject := mkProject(t, owner, src.WS, src.Group, "Остающийся проект")
	stayBoard := mkBoard(t, owner, stayProject, "Доска остающегося")
	stayCols := owner.get("/boards/" + stayBoard + "/columns").listBody(t)
	stayCol := stayCols[0]["id"].(string)

	moving := mkTask(t, owner, src.Board, src.col(t, 0), "переезжающая задача")
	staying := mkTask(t, owner, stayBoard, stayCol, "остающаяся задача")
	// A second task in the moving project: its tag comes from the same project
	// and crosses no boundary, so the transfer must leave it alone.
	travelling := mkTask(t, owner, src.Board, src.col(t, 0), "её попутчица")

	movingTag := owner.expect(t, owner.post("/projects/"+src.Project+"/tags",
		map[string]any{"name": "тег переезжающего", "color": "#7c5cff"}), http.StatusCreated)
	stayTag := owner.expect(t, owner.post("/projects/"+stayProject+"/tags",
		map[string]any{"name": "тег остающегося", "color": "#ff5c7c"}), http.StatusCreated)

	// Both directions, so a one-sided cleanup cannot pass.
	owner.expect(t, owner.post("/tasks/"+moving["id"].(string)+"/tags",
		map[string]any{"tag_id": stayTag["id"]}), http.StatusNoContent)
	owner.expect(t, owner.post("/tasks/"+staying["id"].(string)+"/tags",
		map[string]any{"tag_id": movingTag["id"]}), http.StatusNoContent)
	// The control: tag and task travel together.
	owner.expect(t, owner.post("/tasks/"+travelling["id"].(string)+"/tags",
		map[string]any{"tag_id": movingTag["id"]}), http.StatusNoContent)

	res := owner.expect(t, owner.post("/projects/"+src.Project+"/transfer",
		map[string]any{"workspace_id": dst.WS}), http.StatusOK)

	taskTagIDs := func(t *testing.T, taskID string) []string {
		t.Helper()
		detail := owner.expect(t, owner.get("/tasks/"+taskID), http.StatusOK)
		raw, _ := detail["tags"].([]any)
		out := make([]string, 0, len(raw))
		for _, r := range raw {
			m, _ := r.(map[string]any)
			id, _ := m["id"].(string)
			out = append(out, id)
		}
		return out
	}

	t.Run("the transfer reports what it stripped", func(t *testing.T) {
		if n, _ := res["stripped_tags"].(float64); n != 2 {
			t.Fatalf("stripped_tags = %v, want 2: %v", res["stripped_tags"], res)
		}
	})

	t.Run("no tag survives the boundary", func(t *testing.T) {
		for _, id := range []string{moving["id"].(string), staying["id"].(string)} {
			if got := taskTagIDs(t, id); len(got) != 0 {
				t.Fatalf("task %s still carries a tag from another workspace: %v", id, got)
			}
		}
	})

	t.Run("a tag that travelled with its task is untouched", func(t *testing.T) {
		got := taskTagIDs(t, travelling["id"].(string))
		if len(got) != 1 || got[0] != movingTag["id"] {
			t.Fatalf("travelling task lost its own project's tag: %v", got)
		}
	})

	t.Run("the tags themselves survive, each in its own workspace", func(t *testing.T) {
		want := map[string]string{
			movingTag["id"].(string): dst.WS,
			stayTag["id"].(string):   src.WS,
		}
		for id, ws := range want {
			found := false
			for _, tg := range owner.get("/workspaces/" + ws + "/tags").listBody(t) {
				if tg["id"] == id {
					found = true
				}
			}
			if !found {
				t.Fatalf("tag %s is missing from workspace %s — the cleanup deleted the tag, not the pairing", id, ws)
			}
		}
	})
}
