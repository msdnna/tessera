// Команды-макросы и шаблоны задач (#3069): shared vs personal, who edits what,
// a trigger answers once, links are http(s), templates are the workspace's.
package main

import (
	"net/http"
	"testing"
)

func TestMacrosAndTemplates(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	mate := signup(t)
	ws := mkWorkspace(t, owner, "WS macros")
	owner.expect(t, owner.post("/workspaces/"+ws+"/members",
		map[string]any{"email": mate.Email, "role": "member"}), http.StatusCreated)

	// A template, then a shared macro that makes tasks from it.
	tpl := owner.expect(t, owner.post("/workspaces/"+ws+"/task-templates", map[string]any{
		"name": "Баг-репорт", "title": "Баг: ", "description": "## Шаги\n- [ ] воспроизвести",
		"tags": []string{"type::bug", " ", "срочно"}, "priority": 3, "subtasks": []string{"Тест", ""},
	}), http.StatusCreated)
	if tags, _ := tpl["tags"].([]any); len(tags) != 2 {
		t.Fatalf("template tags not cleaned: %v", tpl["tags"])
	}
	bug := owner.expect(t, owner.post("/workspaces/"+ws+"/macros", map[string]any{
		"name": "Новый баг", "trigger": ">Bug", "action": "create_task", "shared": true,
		"params": map[string]any{"template_id": tpl["id"]}, "keys": "Alt+KeyB", "category": "Создание задач",
	}), http.StatusCreated)
	if bug["trigger"] != "bug" || bug["shared"] != true {
		t.Fatalf("macro: %v", bug)
	}

	// A member can't make or edit shared ones, but has personal ones of their own.
	if r := mate.post("/workspaces/"+ws+"/macros", map[string]any{
		"name": "x", "trigger": "x", "action": "open_link", "shared": true,
		"params": map[string]any{"url": "https://example.com"},
	}); r.Status != http.StatusForbidden {
		t.Fatalf("member making a shared macro: %d", r.Status)
	}
	if r := mate.patch("/macros/"+bug["id"].(string), map[string]any{
		"name": "y", "trigger": "bug", "action": "create_task", "shared": true,
	}); r.Status != http.StatusForbidden {
		t.Fatalf("member editing a shared macro: %d", r.Status)
	}
	if r := mate.post("/workspaces/"+ws+"/macros", map[string]any{
		"name": "Мой баг", "trigger": "bug", "action": "create_task",
	}); r.Status != http.StatusConflict {
		t.Fatalf("trigger taken by a shared macro: %d", r.Status)
	}
	if r := mate.post("/workspaces/"+ws+"/macros", map[string]any{
		"name": "Плохая ссылка", "trigger": "bad", "action": "open_link",
		"params": map[string]any{"url": "javascript:alert(1)"},
	}); r.Status != http.StatusBadRequest {
		t.Fatalf("a javascript: link: %d", r.Status)
	}
	graf := mate.expect(t, mate.post("/workspaces/"+ws+"/macros", map[string]any{
		"name": "Grafana", "trigger": "graf", "action": "open_link",
		"params": map[string]any{"url": "https://grafana.example.com/d/api"},
	}), http.StatusCreated)

	// Personal macros are their owner's alone.
	list := owner.expect(t, owner.get("/workspaces/"+ws+"/macros"), http.StatusOK)
	if ms, _ := list["macros"].([]any); len(ms) != 1 {
		t.Fatalf("owner sees %d macros, want only the shared one", len(ms))
	}
	if r := owner.del("/macros/" + graf["id"].(string)); r.Status != http.StatusNotFound {
		t.Fatalf("someone else's personal macro: %d, want 404", r.Status)
	}
	list = mate.expect(t, mate.get("/workspaces/"+ws+"/macros"), http.StatusOK)
	if ms, _ := list["macros"].([]any); len(ms) != 2 || list["can_manage"] != false {
		t.Fatalf("mate's list: %v", list)
	}
	mate.expect(t, mate.del("/macros/"+graf["id"].(string)), http.StatusNoContent)

	// Templates: any member reads; the author or an admin edits.
	if r := mate.patch("/task-templates/"+tpl["id"].(string), map[string]any{"name": "Чужой"}); r.Status != http.StatusForbidden {
		t.Fatalf("member editing another's template: %d", r.Status)
	}
	if got := mate.get("/workspaces/" + ws + "/task-templates").listBody(t); len(got) != 1 {
		t.Fatalf("templates: %v", got)
	}
	owner.expect(t, owner.del("/task-templates/"+tpl["id"].(string)), http.StatusNoContent)
}
