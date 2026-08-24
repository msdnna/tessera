// The default names the server seeds — the four board columns and the personal
// workspace — travel with a language-neutral key next to the stored Russian
// string, so a client can caption them in the reader's language (#2800). What the
// tests below pin is the boundary that makes the key trustworthy: it marks a name
// the server chose, and a rename by the user takes it away.
package main

import (
	"net/http"
	"testing"
)

// nameKey reads the name_key field of a JSON object: "" when absent or null,
// which is exactly how a client tells "server default" from "user's own name".
func nameKey(t *testing.T, m map[string]any) string {
	t.Helper()
	v, ok := m["name_key"]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("name_key is %T, want string: %v", v, m)
	}
	return s
}

func TestColumnNameKeys(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	// The seeded four carry their keys, in board order.
	want := []string{"todo", "in_progress", "review", "done"}
	for i, k := range want {
		if got := nameKey(t, s.Columns[i]); got != k {
			t.Fatalf("seeded column %d: name_key %q, want %q", i, got, k)
		}
	}

	// A column the user adds is their own name from the start — no key, so it is
	// shown verbatim in every language.
	col := c.expect(t, c.post("/boards/"+s.Board+"/columns",
		map[string]any{"name": "Новая", "color": "#123456"}), http.StatusCreated)
	newID := col["id"].(string)
	if got := nameKey(t, col); got != "" {
		t.Fatalf("user-added column: name_key %q, want empty", got)
	}

	// Re-saving a seeded column with the same name is not a rename — the update
	// endpoint is full-replace, so a colour edit resends the name and must not
	// cost the column its key.
	todoID := s.col(t, 0)
	upd := c.expect(t, c.patch("/columns/"+todoID,
		map[string]any{"name": s.Columns[0]["name"], "color": "#111111"}), http.StatusOK)
	if got := nameKey(t, upd); got != "todo" {
		t.Fatalf("colour-only update: name_key %q, want todo", got)
	}

	// An actual rename drops the key: a column someone called «Бэклог» must stay
	// «Бэклог» on an English UI, not turn back into "To do".
	upd = c.expect(t, c.patch("/columns/"+todoID,
		map[string]any{"name": "Бэклог", "color": "#111111"}), http.StatusOK)
	if got := nameKey(t, upd); got != "" {
		t.Fatalf("renamed column: name_key %q, want empty", got)
	}
	cols := c.get("/boards/" + s.Board + "/columns").listBody(t)
	if got := nameKey(t, byID(t, cols, todoID)); got != "" {
		t.Fatalf("renamed column after reload: name_key %q, want empty", got)
	}
	// The other seeded columns are untouched by that rename.
	if got := nameKey(t, byID(t, cols, s.col(t, 1))); got != "in_progress" {
		t.Fatalf("sibling column: name_key %q, want in_progress", got)
	}
	if got := nameKey(t, byID(t, cols, newID)); got != "" {
		t.Fatalf("user-added column after reload: name_key %q, want empty", got)
	}

	// A task carries its column's key too — the home screen shows the column
	// without loading the board.
	task := mkTask(t, c, s.Board, s.col(t, 2), "Задача в колонке")
	tasks := c.get("/workspaces/" + s.WS + "/tasks").listBody(t)
	row := byID(t, tasks, task["id"].(string))
	if row["column_name_key"] != "review" {
		t.Fatalf("task row column_name_key = %v, want review", row["column_name_key"])
	}
	if row["column_name"] != "На рассмотрении" {
		t.Fatalf("task row column_name = %v, want the stored name", row["column_name"])
	}
}

func TestWorkspaceNameKey(t *testing.T) {
	t.Parallel()
	c := signup(t)

	// Registration seeds one workspace, and it is the keyed one.
	list := c.get("/workspaces").listBody(t)
	if len(list) != 1 {
		t.Fatalf("fresh account has %d workspaces, want 1", len(list))
	}
	personal := list[0]
	if got := nameKey(t, personal); got != "personal" {
		t.Fatalf("personal workspace: name_key %q, want personal", got)
	}
	if personal["name"] != "Личное пространство" {
		t.Fatalf("personal workspace name = %v, want the stored fallback", personal["name"])
	}
	personalID := personal["id"].(string)

	// A workspace the user creates is named by them — no key.
	own := c.expect(t, c.post("/workspaces", map[string]any{"name": "Общее"}), http.StatusCreated)
	if got := nameKey(t, own); got != "" {
		t.Fatalf("user-created workspace: name_key %q, want empty", got)
	}

	// Re-saving the same name keeps the key; renaming drops it for good.
	upd := c.expect(t, c.patch("/workspaces/"+personalID,
		map[string]any{"name": "Личное пространство"}), http.StatusOK)
	if got := nameKey(t, upd); got != "personal" {
		t.Fatalf("workspace re-save: name_key %q, want personal", got)
	}
	upd = c.expect(t, c.patch("/workspaces/"+personalID,
		map[string]any{"name": "Мой уголок"}), http.StatusOK)
	if got := nameKey(t, upd); got != "" {
		t.Fatalf("renamed workspace: name_key %q, want empty", got)
	}
	if got := nameKey(t, c.expect(t, c.get("/workspaces/"+personalID), http.StatusOK)); got != "" {
		t.Fatalf("renamed workspace after reload: name_key %q, want empty", got)
	}
}
