package main

import (
	"net/http"
	"testing"
)

// Project groups: create (flat + nested), list, update, move, delete.
func TestProjectGroupFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS groups")

	root := mkGroup(t, c, ws, "Корень")
	// Nested group via parent_id.
	sub := c.expect(t, c.post("/workspaces/"+ws+"/groups",
		map[string]any{"name": "Вложенная", "parent_id": root}), http.StatusCreated)
	if sub["parent_id"] != root {
		t.Fatalf("nested parent_id = %v, want %s", sub["parent_id"], root)
	}
	other := mkGroup(t, c, ws, "Соседняя")

	list := c.get("/workspaces/" + ws + "/groups").listBody(t)
	if len(list) != 3 {
		t.Fatalf("groups = %d, want 3", len(list))
	}

	upd := c.expect(t, c.patch("/groups/"+root, map[string]any{
		"name": "Корень 2", "icon": "folder", "color": "#7c5cff", "icon_mode": "icon",
	}), http.StatusOK)
	if upd["name"] != "Корень 2" || upd["icon_mode"] != "icon" {
		t.Fatalf("group update: %v", upd)
	}

	// Re-parent "Вложенная" under "Соседняя".
	moved := c.expect(t, c.patch("/groups/"+sub["id"].(string)+"/move",
		map[string]any{"parent_id": other}), http.StatusOK)
	if moved["parent_id"] != other {
		t.Fatalf("group move parent = %v, want %s", moved["parent_id"], other)
	}
	// A group cannot be its own parent.
	if r := c.patch("/groups/"+root+"/move", map[string]any{"parent_id": root}); r.Status != http.StatusBadRequest {
		t.Fatalf("self-parent: status %d", r.Status)
	}

	c.expect(t, c.del("/groups/"+other), http.StatusNoContent)
	// Subgroups cascade with their parent.
	list = c.get("/workspaces/" + ws + "/groups").listBody(t)
	if len(list) != 1 {
		t.Fatalf("groups after delete = %d, want 1 (subgroup should cascade)", len(list))
	}
}

// Projects: create/list/get/update/move/delete.
func TestProjectFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS projects")
	g1 := mkGroup(t, c, ws, "Группа 1")
	g2 := mkGroup(t, c, ws, "Группа 2")

	p := c.expect(t, c.post("/workspaces/"+ws+"/projects",
		map[string]any{"name": "Проект Альфа", "group_id": g1, "color": "#ff0000", "icon": "rocket"}), http.StatusCreated)
	pID := p["id"].(string)
	if p["group_id"] != g1 || p["slug"] == "" {
		t.Fatalf("project create: %v", p)
	}

	if got := len(c.get("/workspaces/" + ws + "/projects").listBody(t)); got != 1 {
		t.Fatalf("projects = %d, want 1", got)
	}
	got := c.expect(t, c.get("/projects/"+pID), http.StatusOK)
	if got["name"] != "Проект Альфа" {
		t.Fatalf("project get: %v", got)
	}

	// Update name/icon/color/icon_mode (unknown icon_mode falls back to "badge").
	upd := c.expect(t, c.patch("/projects/"+pID, map[string]any{
		"name": "Проект Бета", "color": "#00ff00", "icon": "flame", "icon_mode": "icon", "group_id": g1,
	}), http.StatusOK)
	if upd["name"] != "Проект Бета" || upd["color"] != "#00ff00" || upd["icon_mode"] != "icon" {
		t.Fatalf("project update: %v", upd)
	}

	// Move to another group.
	moved := c.expect(t, c.patch("/projects/"+pID+"/move", map[string]any{"group_id": g2}), http.StatusOK)
	if moved["group_id"] != g2 {
		t.Fatalf("project move group = %v, want %s", moved["group_id"], g2)
	}

	c.expect(t, c.del("/projects/"+pID), http.StatusNoContent)
	c.expect(t, c.get("/projects/"+pID), http.StatusNotFound)
}

// Transfer moves a project (boards, tasks, tags) to another workspace of the
// same user, re-stamps tag workspace_ids and strips non-member assignees.
func TestProjectTransfer(t *testing.T) {
	t.Parallel()
	c := signup(t)
	helper := signup(t)
	s := mkStack(t, c) // source workspace
	ws2 := mkWorkspace(t, c, "WS transfer target")

	// helper is a member of the SOURCE workspace only.
	c.expect(t, c.post("/workspaces/"+s.WS+"/members", map[string]any{"email": helper.Email}), http.StatusCreated)

	task := mkTask(t, c, s.Board, s.col(t, 0), "Переезжающая задача")
	taskID := task["id"].(string)
	tag := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "переезд", "color": "#123456"}), http.StatusCreated)
	c.expect(t, c.post("/tasks/"+taskID+"/tags", map[string]any{"tag_id": tag["id"]}), http.StatusNoContent)
	c.expect(t, c.post("/tasks/"+taskID+"/assignees", map[string]any{"user_id": helper.UserID}), http.StatusNoContent)

	// Transfer into the same workspace → 400.
	if r := c.post("/projects/"+s.Project+"/transfer", map[string]any{"workspace_id": s.WS}); r.Status != http.StatusBadRequest {
		t.Fatalf("transfer to same ws: status %d", r.Status)
	}

	out := c.expect(t, c.post("/projects/"+s.Project+"/transfer",
		map[string]any{"workspace_id": ws2}), http.StatusOK)
	proj := out["project"].(map[string]any)
	if proj["workspace_id"] != ws2 {
		t.Fatalf("project workspace after transfer = %v, want %s", proj["workspace_id"], ws2)
	}
	if proj["group_id"] != nil {
		t.Fatalf("transferred project should land ungrouped, got %v", proj["group_id"])
	}

	// The task is reachable through the target workspace.
	if !listHasID(c.get("/workspaces/"+ws2+"/tasks").listBody(t), taskID) {
		t.Fatalf("task not visible in target workspace")
	}
	// Tags were re-stamped to the target workspace.
	if !listHasID(c.get("/workspaces/"+ws2+"/tags").listBody(t), tag["id"].(string)) {
		t.Fatalf("tag not re-stamped to target workspace")
	}
	if listHasID(c.get("/workspaces/"+s.WS+"/tags").listBody(t), tag["id"].(string)) {
		t.Fatalf("tag still listed in source workspace")
	}
	// helper is not a member of ws2 → stripped from assignees.
	detail := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if assignees, ok := detail["assignees"].([]any); ok && len(assignees) != 0 {
		t.Fatalf("non-member assignee not stripped: %v", assignees)
	}
	// helper (source-only member) lost access to the task.
	helper.expect(t, helper.get("/tasks/"+taskID), http.StatusForbidden)
}

// Slug resolution: /board-by-slug?project=<slug>&board=<slug>.
func TestBoardBySlug(t *testing.T) {
	t.Parallel()
	c := signup(t)
	ws := mkWorkspace(t, c, "WS slugs")
	g := mkGroup(t, c, ws, "Группа")
	p := c.expect(t, c.post("/workspaces/"+ws+"/projects",
		map[string]any{"name": "Slug Project", "group_id": g}), http.StatusCreated)
	b := c.expect(t, c.post("/projects/"+p["id"].(string)+"/boards",
		map[string]any{"name": "Slug Board"}), http.StatusCreated)

	pSlug, bSlug := p["slug"].(string), b["slug"].(string)
	if pSlug == "" || bSlug == "" {
		t.Fatalf("empty slugs: project=%q board=%q", pSlug, bSlug)
	}

	got := c.expect(t, c.get("/board-by-slug?project="+pSlug+"&board="+bSlug), http.StatusOK)
	if got["id"] != b["id"] {
		t.Fatalf("resolved board %v, want %v", got["id"], b["id"])
	}

	// Unknown project slug and unknown board slug both → 404.
	c.expect(t, c.get("/board-by-slug?project=no-such-project&board="+bSlug), http.StatusNotFound)
	c.expect(t, c.get("/board-by-slug?project="+pSlug+"&board=no-such-board"), http.StatusNotFound)

	// A non-member can't resolve someone else's slugs.
	other := signup(t)
	other.expect(t, other.get("/board-by-slug?project="+pSlug+"&board="+bSlug), http.StatusForbidden)
}

// Estimation config: workspace default + project override, reflected in GETs.
func TestEstimationConfig(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	// Workspace-level "time" config; out-of-range hours are clamped server-side.
	ws := c.expect(t, c.put("/workspaces/"+s.WS+"/estimation",
		map[string]any{"unit": "time", "hours_per_day": 6, "days_per_week": 4}), http.StatusOK)
	est, _ := ws["estimation"].(map[string]any)
	if est == nil || est["unit"] != "time" || est["hours_per_day"] != float64(6) || est["days_per_week"] != float64(4) {
		t.Fatalf("workspace estimation = %v", ws["estimation"])
	}
	// Reflected in GET /workspaces/:id.
	got := c.expect(t, c.get("/workspaces/"+s.WS), http.StatusOK)
	if est, _ := got["estimation"].(map[string]any); est == nil || est["unit"] != "time" {
		t.Fatalf("workspace GET estimation = %v", got["estimation"])
	}

	// Project override: points/tshirt.
	p := c.expect(t, c.put("/projects/"+s.Project+"/estimation",
		map[string]any{"unit": "points", "points_scale": "tshirt"}), http.StatusOK)
	if est, _ := p["estimation"].(map[string]any); est == nil || est["unit"] != "points" || est["points_scale"] != "tshirt" {
		t.Fatalf("project estimation = %v", p["estimation"])
	}
	got = c.expect(t, c.get("/projects/"+s.Project), http.StatusOK)
	if est, _ := got["estimation"].(map[string]any); est == nil || est["points_scale"] != "tshirt" {
		t.Fatalf("project GET estimation = %v", got["estimation"])
	}

	// Invalid unit → 400.
	if r := c.put("/workspaces/"+s.WS+"/estimation", map[string]any{"unit": "bananas"}); r.Status != http.StatusBadRequest {
		t.Fatalf("invalid unit: status %d\n%s", r.Status, r.Body)
	}

	// Empty body clears the override back to inherit (estimation → null).
	cleared := c.expect(t, c.put("/projects/"+s.Project+"/estimation", nil), http.StatusOK)
	if cleared["estimation"] != nil {
		t.Fatalf("cleared project estimation = %v, want null", cleared["estimation"])
	}
}
