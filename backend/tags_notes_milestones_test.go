package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// Tags: project-scoped CRUD, workspace aggregate, attach/detach on tasks.
func TestTagFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	tag := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "bug", "color": "#ff0000"}), http.StatusCreated)
	tagID := tag["id"].(string)
	if tag["project_id"] != s.Project || tag["workspace_id"] != s.WS {
		t.Fatalf("tag scoping: %v", tag)
	}

	// Duplicate name in the same project violates UNIQUE(project_id, name).
	// FACT: the handler maps it to a generic 500 (no 409 branch) — see report.
	if r := c.post("/projects/"+s.Project+"/tags", map[string]any{"name": "bug"}); r.Status != http.StatusInternalServerError {
		t.Fatalf("duplicate tag: status %d, expected current behavior 500\n%s", r.Status, r.Body)
	}

	c.expect(t, c.post("/projects/"+s.Project+"/tags", map[string]any{"name": "feature"}), http.StatusCreated)
	if got := len(c.get("/projects/" + s.Project + "/tags").listBody(t)); got != 2 {
		t.Fatalf("tags = %d, want 2", got)
	}

	// Rename + recolor.
	upd := c.expect(t, c.patch("/tags/"+tagID, map[string]any{"name": "defect", "color": "#00ff00"}), http.StatusOK)
	if upd["name"] != "defect" || upd["color"] != "#00ff00" {
		t.Fatalf("tag update: %v", upd)
	}

	// Workspace-wide read-only aggregate spans projects.
	g2 := mkGroup(t, c, s.WS, "Группа 2")
	p2 := mkProject(t, c, s.WS, g2, "Проект 2")
	c.expect(t, c.post("/projects/"+p2+"/tags", map[string]any{"name": "p2-tag"}), http.StatusCreated)
	if got := len(c.get("/workspaces/" + s.WS + "/tags").listBody(t)); got != 3 {
		t.Fatalf("workspace tags = %d, want 3", got)
	}

	// Attach/detach on a task.
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача с тегами")
	taskID := task["id"].(string)
	c.expect(t, c.post("/tasks/"+taskID+"/tags", map[string]any{"tag_id": tagID}), http.StatusNoContent)
	detail := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if tags, _ := detail["tags"].([]any); len(tags) != 1 {
		t.Fatalf("task tags after attach: %v", detail["tags"])
	}

	// Tag from ANOTHER project of the same workspace.
	// FACT: AddTaskTag does no project-scope validation — the foreign tag
	// attaches successfully (204). Recorded as a bug in the report.
	other := c.get("/projects/" + p2 + "/tags").listBody(t)
	c.expect(t, c.post("/tasks/"+taskID+"/tags", map[string]any{"tag_id": other[0]["id"]}), http.StatusNoContent)
	detail = c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if tags, _ := detail["tags"].([]any); len(tags) != 2 {
		t.Fatalf("cross-project tag: expected current behavior (attached), got %v", detail["tags"])
	}
	c.expect(t, c.del("/tasks/"+taskID+"/tags/"+other[0]["id"].(string)), http.StatusNoContent)

	c.expect(t, c.del("/tasks/"+taskID+"/tags/"+tagID), http.StatusNoContent)
	detail = c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if tags, _ := detail["tags"].([]any); len(tags) != 0 {
		t.Fatalf("task tags after detach: %v", detail["tags"])
	}

	// Delete a tag (cascades off tasks).
	c.expect(t, c.del("/tags/"+tagID), http.StatusNoContent)
	if got := len(c.get("/projects/" + s.Project + "/tags").listBody(t)); got != 1 {
		t.Fatalf("tags after delete = %d, want 1", got)
	}
}

// Tag prefixes: PUT replaces the full set (canonicalised keys), GET lists it.
func TestTagPrefixes(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	r := c.put("/projects/"+s.Project+"/tag-prefixes", map[string]any{
		"prefixes": []map[string]any{
			{"prefix": " S: ", "label": "Размер"},
			{"prefix": "prio:", "label": "Приоритет"},
			{"prefix": "s:", "label": "дубль — отбрасывается"}, // same canonical key
			{"prefix": "", "label": "без ключа — отбрасывается"},
		},
	})
	if r.Status != http.StatusOK {
		t.Fatalf("set prefixes: status %d\n%s", r.Status, r.Body)
	}
	set := r.listBody(t)
	if len(set) != 2 {
		t.Fatalf("stored prefixes = %d, want 2 (dedup + blank drop)\n%v", len(set), set)
	}

	list := c.get("/projects/" + s.Project + "/tag-prefixes").listBody(t)
	byPrefix := map[string]string{}
	for _, p := range list {
		byPrefix[p["prefix"].(string)] = p["label"].(string)
	}
	// Keys are canonicalised: trimmed + lowercased.
	if byPrefix["s:"] != "Размер" || byPrefix["prio:"] != "Приоритет" {
		t.Fatalf("prefixes: %v", byPrefix)
	}

	// PUT is full-replace: a shorter set removes the rest.
	c.put("/projects/"+s.Project+"/tag-prefixes", map[string]any{
		"prefixes": []map[string]any{{"prefix": "prio:", "label": "Prio"}},
	})
	if got := len(c.get("/projects/" + s.Project + "/tag-prefixes").listBody(t)); got != 1 {
		t.Fatalf("prefixes after replace = %d, want 1", got)
	}
}

// Workspace-wide prefix list: the union across the workspace's projects, deduped
// by canonical prefix so cross-project views (Home) get one label per scope.
func TestWorkspaceTagPrefixes(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	p2 := mkProject(t, c, s.WS, s.Group, "Второй проект "+t.Name())

	c.put("/projects/"+s.Project+"/tag-prefixes", map[string]any{
		"prefixes": []map[string]any{
			{"prefix": "effort::", "label": "Сложность"},
			{"prefix": "T:", "label": "Тип"},
		},
	})
	// Second project repeats one prefix (a different label) and adds one of its own.
	c.put("/projects/"+p2+"/tag-prefixes", map[string]any{
		"prefixes": []map[string]any{
			{"prefix": "effort::", "label": "Другая подпись"},
			{"prefix": "area::", "label": "Область"},
		},
	})

	r := c.get("/workspaces/" + s.WS + "/tag-prefixes")
	if r.Status != http.StatusOK {
		t.Fatalf("list ws prefixes: status %d\n%s", r.Status, r.Body)
	}
	byPrefix := map[string]string{}
	for _, p := range r.listBody(t) {
		key := p["prefix"].(string)
		if _, dup := byPrefix[key]; dup {
			t.Fatalf("prefix %q listed twice — not deduped", key)
		}
		byPrefix[key] = p["label"].(string)
	}
	if len(byPrefix) != 3 {
		t.Fatalf("ws prefixes = %d, want 3\n%v", len(byPrefix), byPrefix)
	}
	if byPrefix["t:"] != "Тип" || byPrefix["area::"] != "Область" {
		t.Fatalf("ws prefixes: %v", byPrefix)
	}
	// The repeated prefix keeps exactly one of the two labels (first wins).
	if l := byPrefix["effort::"]; l != "Сложность" && l != "Другая подпись" {
		t.Fatalf("effort:: label = %q, want one of the two project labels", l)
	}
}

// Notes: workspace-scoped CRUD.
func TestNoteFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	note := c.expect(t, c.post("/workspaces/"+s.WS+"/notes",
		map[string]any{"title": "Идеи", "body": "первый черновик", "project_id": s.Project}), http.StatusCreated)
	noteID := note["id"].(string)
	if note["slug"] == "" || note["project_id"] != s.Project {
		t.Fatalf("note create: %v", note)
	}
	// A second note without a project.
	c.expect(t, c.post("/workspaces/"+s.WS+"/notes", map[string]any{"title": "Без проекта"}), http.StatusCreated)

	if got := len(c.get("/workspaces/" + s.WS + "/notes").listBody(t)); got != 2 {
		t.Fatalf("notes = %d, want 2", got)
	}

	got := c.expect(t, c.get("/notes/"+noteID), http.StatusOK)
	if got["body"] != "первый черновик" {
		t.Fatalf("note get: %v", got)
	}

	upd := c.expect(t, c.patch("/notes/"+noteID, map[string]any{"title": "Идеи v2", "body": "правка"}), http.StatusOK)
	if upd["title"] != "Идеи v2" || upd["body"] != "правка" {
		t.Fatalf("note update: %v", upd)
	}

	// Notes are workspace-guarded.
	other := signup(t)
	other.expect(t, other.get("/notes/"+noteID), http.StatusForbidden)

	c.expect(t, c.del("/notes/"+noteID), http.StatusNoContent)
	c.expect(t, c.get("/notes/"+noteID), http.StatusNotFound)
}

// Milestones: project CRUD, workspace rollup list, assignment to tasks.
func TestMilestoneFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	m := c.expect(t, c.post("/projects/"+s.Project+"/milestones", map[string]any{
		"title": "Этап 1", "description": "первый",
		"start_date": "2026-08-01T00:00:00Z", "due_date": "2026-08-31T00:00:00Z",
	}), http.StatusCreated)
	msID := m["id"].(string)
	if m["state"] != "active" || m["start_date"] == nil || m["due_date"] == nil {
		t.Fatalf("milestone create: %v", m)
	}
	// Title is required.
	if r := c.post("/projects/"+s.Project+"/milestones", map[string]any{"description": "без названия"}); r.Status != http.StatusBadRequest {
		t.Fatalf("milestone without title: status %d", r.Status)
	}

	if got := len(c.get("/projects/" + s.Project + "/milestones").listBody(t)); got != 1 {
		t.Fatalf("project milestones = %d, want 1", got)
	}

	// Assign to a task, verify, then clear.
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача этапа")
	taskID := task["id"].(string)
	c.expect(t, c.post("/tasks/"+taskID+"/milestone", map[string]any{"milestone_id": msID}), http.StatusNoContent)
	detail := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if detail["milestone_id"] != msID {
		t.Fatalf("task milestone_id = %v, want %s", detail["milestone_id"], msID)
	}

	// Workspace-wide list carries task rollups.
	wsList := c.get("/workspaces/" + s.WS + "/milestones").listBody(t)
	if len(wsList) != 1 || wsList[0]["id"] != msID {
		t.Fatalf("workspace milestones: %v", wsList)
	}

	// Update (full payload; state flips to closed).
	upd := c.expect(t, c.patch("/milestones/"+msID, map[string]any{
		"title": "Этап 1 (закрыт)", "state": "closed",
	}), http.StatusOK)
	if upd["title"] != "Этап 1 (закрыт)" || upd["state"] != "closed" {
		t.Fatalf("milestone update: %v", upd)
	}

	c.expect(t, c.del("/tasks/"+taskID+"/milestone"), http.StatusNoContent)
	detail = c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if detail["milestone_id"] != nil {
		t.Fatalf("task milestone after clear = %v", detail["milestone_id"])
	}

	// Re-assign, then delete the milestone — the task's pointer is SET NULL.
	c.expect(t, c.post("/tasks/"+taskID+"/milestone", map[string]any{"milestone_id": msID}), http.StatusNoContent)
	c.expect(t, c.del("/milestones/"+msID), http.StatusNoContent)
	detail = c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
	if detail["milestone_id"] != nil {
		t.Fatalf("task milestone after milestone delete = %v", detail["milestone_id"])
	}
	if got := len(c.get("/projects/" + s.Project + "/milestones").listBody(t)); got != 0 {
		t.Fatalf("milestones after delete = %d, want 0", got)
	}
}

// Milestone slugs: assigned on create, unique per project, "backlog" reserved,
// and usable in ?milestone= alongside the legacy UUID form.
func TestMilestoneSlugScope(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	create := func(title string) map[string]any {
		return c.expect(t, c.post("/projects/"+s.Project+"/milestones",
			map[string]any{"title": title}), http.StatusCreated)
	}

	first := create("Спринт 1")
	if first["slug"] != "sprint-1" {
		t.Fatalf("slug = %v, want sprint-1", first["slug"])
	}
	// Same title in the same project → suffixed, not a unique-index error.
	if dup := create("Спринт 1"); dup["slug"] != "sprint-1-2" {
		t.Fatalf("duplicate slug = %v, want sprint-1-2", dup["slug"])
	}
	// "backlog" is the reserved no-milestone scope, so it never gets handed out.
	if bl := create("Backlog"); bl["slug"] != "backlog-2" {
		t.Fatalf("Backlog slug = %v, want backlog-2", bl["slug"])
	}

	// The workspace roadmap list needs the slug too — it builds the deep link.
	wsList := c.get("/workspaces/" + s.WS + "/milestones").listBody(t)
	if len(wsList) == 0 || wsList[0]["slug"] == nil || wsList[0]["slug"] == "" {
		t.Fatalf("workspace milestones carry no slug: %v", wsList)
	}

	msID := first["id"].(string)
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача спринта")
	c.expect(t, c.post("/tasks/"+task["id"].(string)+"/milestone",
		map[string]any{"milestone_id": msID}), http.StatusNoContent)
	// A second task stays in the backlog, so scope filtering is observable.
	mkTask(t, c, s.Board, s.col(t, 0), "Задача без этапа")

	scoped := func(scope string) []map[string]any {
		return c.get("/boards/" + s.Board + "/tasks?milestone=" + scope).listBody(t)
	}
	bySlug, byUUID := scoped("sprint-1"), scoped(msID)
	if len(bySlug) != 1 || len(byUUID) != 1 || bySlug[0]["id"] != byUUID[0]["id"] {
		t.Fatalf("slug scope %v != uuid scope %v", bySlug, byUUID)
	}
	if got := len(scoped(backlogScopeTest)); got != 1 {
		t.Fatalf("backlog scope = %d tasks, want 1", got)
	}
	// A stale/broken link shows an empty scoped board, not the whole board.
	if got := len(scoped("net-takogo-etapa")); got != 0 {
		t.Fatalf("unknown slug scope = %d tasks, want 0", got)
	}
	if got := len(c.get("/boards/" + s.Board + "/tasks").listBody(t)); got != 2 {
		t.Fatalf("unscoped board = %d tasks, want 2", got)
	}
}

// backlogScopeTest mirrors handlers.backlogScope (different package under test).
const backlogScopeTest = "backlog"

// Search: task titles/descriptions + note titles/bodies within a workspace.
func TestWorkspaceSearch(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	marker := "srchmarker" + s.WS[:8]
	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача "+marker)
	c.expect(t, c.post("/workspaces/"+s.WS+"/notes",
		map[string]any{"title": "Заметка " + marker}), http.StatusCreated)
	mkTask(t, c, s.Board, s.col(t, 0), "Нерелевантная задача")

	res := c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+marker), http.StatusOK)
	tasks, _ := res["tasks"].([]any)
	notes, _ := res["notes"].([]any)
	if len(tasks) != 1 || len(notes) != 1 {
		t.Fatalf("search hits: tasks=%d notes=%d\n%v", len(tasks), len(notes), res)
	}
	if tasks[0].(map[string]any)["id"] != task["id"] {
		t.Fatalf("search task id mismatch: %v", tasks[0])
	}

	// Empty query short-circuits to empty result sets.
	res = c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="), http.StatusOK)
	if len(res["tasks"].([]any)) != 0 || len(res["notes"].([]any)) != 0 {
		t.Fatalf("empty query: %v", res)
	}
}

// Search matches a case-insensitive substring anywhere in the searched columns,
// including a hit that exists only in a task description or a note body. This
// pins the semantics the trigram indexes (migration 0052) must preserve: they
// speed up the same leading-wildcard ILIKE, they do not narrow it into a
// word-prefix or whole-word match the way an FTS rewrite would.
func TestWorkspaceSearchMatchesSubstringAndBodies(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	// Mixed case on the way in, lower case on the way out — the marker sits in
	// the middle of a longer word, so a prefix-only match would miss it.
	marker := "SrchDeep" + s.WS[:8]
	query := strings.ToLower(marker)

	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача без маркера в заголовке")
	c.expect(t, c.patch("/tasks/"+task["id"].(string),
		map[string]any{"description": "префикс" + marker + "суффикс"}), http.StatusOK)
	c.expect(t, c.post("/workspaces/"+s.WS+"/notes",
		map[string]any{"title": "Заметка без маркера", "body": "префикс" + marker + "суффикс"}),
		http.StatusCreated)

	res := c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+query), http.StatusOK)
	tasks, _ := res["tasks"].([]any)
	notes, _ := res["notes"].([]any)
	if len(tasks) != 1 || len(notes) != 1 {
		t.Fatalf("substring search hits: tasks=%d notes=%d\n%v", len(tasks), len(notes), res)
	}
	if tasks[0].(map[string]any)["id"] != task["id"] {
		t.Fatalf("substring search task id mismatch: %v", tasks[0])
	}

	// A marker that exists nowhere returns both sets empty, not everything.
	res = c.expect(t, c.get("/workspaces/"+s.WS+"/search?q="+query+"zzz"), http.StatusOK)
	if len(res["tasks"].([]any)) != 0 || len(res["notes"].([]any)) != 0 {
		t.Fatalf("no-hit query: %v", res)
	}
}

// Workspace task list + by-number deep-link resolution.
func TestWorkspaceTasksAndByNumber(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	t1 := mkTask(t, c, s.Board, s.col(t, 0), "Первая")
	t2 := mkTask(t, c, s.Board, s.col(t, 1), "Вторая")

	rows := c.get("/workspaces/" + s.WS + "/tasks").listBody(t)
	if len(rows) != 2 {
		t.Fatalf("workspace tasks = %d, want 2", len(rows))
	}

	// Numbers are per-workspace and start at 1.
	n1, ok1 := t1["number"].(float64)
	n2, ok2 := t2["number"].(float64)
	if !ok1 || !ok2 || n2 != n1+1 {
		t.Fatalf("task numbers: %v %v", t1["number"], t2["number"])
	}

	got := c.expect(t, c.get(fmt.Sprintf("/workspaces/%s/tasks/by-number/%.0f", s.WS, n2)), http.StatusOK)
	if got["id"] != t2["id"] {
		t.Fatalf("by-number resolved %v, want %v", got["id"], t2["id"])
	}

	// Unknown number → 404; junk → 400.
	c.expect(t, c.get("/workspaces/"+s.WS+"/tasks/by-number/999999"), http.StatusNotFound)
	if r := c.get("/workspaces/" + s.WS + "/tasks/by-number/abc"); r.Status != http.StatusBadRequest {
		t.Fatalf("bad number: status %d", r.Status)
	}
}
