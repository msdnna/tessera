// Board lifecycle: CRUD, default/done columns, column management (create,
// rename, move, delete) and saved board views + the board dependency graph.
package main

import (
	"net/http"
	"strings"
	"testing"
)

// ── shared helpers (used by the other flow tests too) ────────────────────────

// byID finds a list item by its "id" field, failing the test when absent.
func byID(t *testing.T, list []map[string]any, id string) map[string]any {
	t.Helper()
	for _, m := range list {
		if m["id"] == id {
			return m
		}
	}
	t.Fatalf("id %s not found in list of %d", id, len(list))
	return nil
}

// hasID reports whether a list contains an item with the given id.
func hasID(list []map[string]any, id string) bool {
	for _, m := range list {
		if m["id"] == id {
			return true
		}
	}
	return false
}

// fpos extracts the float position of an item.
func fpos(t *testing.T, m map[string]any) float64 {
	t.Helper()
	p, ok := m["position"].(float64)
	if !ok {
		t.Fatalf("no float position in %v", m)
	}
	return p
}

// eventKinds returns the journal event kinds of a task, in order.
func eventKinds(t *testing.T, c *client, taskID string) []string {
	t.Helper()
	events := c.get("/tasks/" + taskID + "/events").listBody(t)
	kinds := make([]string, 0, len(events))
	for _, e := range events {
		kinds = append(kinds, e["kind"].(string))
	}
	return kinds
}

func hasKind(kinds []string, k string) bool {
	for _, kk := range kinds {
		if kk == k {
			return true
		}
	}
	return false
}

// ── boards ───────────────────────────────────────────────────────────────────

// Board CRUD: creation is covered by mkStack (with 4 default columns and the
// rightmost column preset as done); here: GET, PATCH (tri-state icon), DELETE.
func TestBoardCRUD(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	// A new board carries the 4 seeded columns in order.
	if len(s.Columns) != 4 {
		t.Fatalf("default columns = %d, want 4", len(s.Columns))
	}
	wantNames := []string{"К работе", "В процессе", "На рассмотрении", "Готово"}
	for i, w := range wantNames {
		if s.Columns[i]["name"] != w {
			t.Fatalf("column %d = %v, want %s", i, s.Columns[i]["name"], w)
		}
	}

	b := c.expect(t, c.get("/boards/"+s.Board), http.StatusOK)
	if b["name"] != "Доска" {
		t.Fatalf("board name = %v", b["name"])
	}
	// The rightmost seeded column ("Готово") is the default done column.
	if b["done_column_id"] != s.col(t, 3) {
		t.Fatalf("done_column_id = %v, want %s", b["done_column_id"], s.col(t, 3))
	}

	// Rename + set icon/color.
	b = c.expect(t, c.patch("/boards/"+s.Board, map[string]any{
		"name": "Доска v2", "icon": "rocket", "color": "#ff8800",
	}), http.StatusOK)
	if b["name"] != "Доска v2" || b["icon"] != "rocket" || b["color"] != "#ff8800" {
		t.Fatalf("board update: %v", b)
	}
	// Tri-state: a rename-only PATCH keeps icon/color.
	b = c.expect(t, c.patch("/boards/"+s.Board, map[string]any{"name": "Доска v3"}), http.StatusOK)
	if b["name"] != "Доска v3" || b["icon"] != "rocket" || b["color"] != "#ff8800" {
		t.Fatalf("rename wiped icon/color: %v", b)
	}

	// Delete → gone.
	if r := c.del("/boards/" + s.Board); r.Status != http.StatusNoContent {
		t.Fatalf("delete board: %d\n%s", r.Status, r.Body)
	}
	if r := c.get("/boards/" + s.Board); r.Status != http.StatusNotFound {
		t.Fatalf("deleted board GET: %d", r.Status)
	}
}

// Done column: explicit configuration + auto complete/reopen on task moves.
func TestBoardDoneColumnFlow(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	// Point the done column at "В процессе" (index 1) explicitly.
	b := c.expect(t, c.patch("/boards/"+s.Board+"/done-column",
		map[string]any{"column_id": s.col(t, 1)}), http.StatusOK)
	if b["done_column_id"] != s.col(t, 1) {
		t.Fatalf("done_column_id = %v, want %s", b["done_column_id"], s.col(t, 1))
	}

	// A column from another board is rejected.
	other := mkStack(t, c)
	if r := c.patch("/boards/"+s.Board+"/done-column",
		map[string]any{"column_id": other.col(t, 0)}); r.Status != http.StatusBadRequest {
		t.Fatalf("foreign done column: %d\n%s", r.Status, r.Body)
	}

	task := mkTask(t, c, s.Board, s.col(t, 0), "Задача")
	id := task["id"].(string)

	// Moving into the done column completes the task + logs "completed".
	moved := c.expect(t, c.patch("/tasks/"+id+"/move",
		map[string]any{"column_id": s.col(t, 1)}), http.StatusOK)
	if moved["completed_at"] == nil {
		t.Fatalf("move to done: completed_at not set: %v", moved)
	}
	kinds := eventKinds(t, c, id)
	if !hasKind(kinds, "completed") || !hasKind(kinds, "moved") {
		t.Fatalf("events after done move: %v", kinds)
	}

	// Moving back out reopens it + logs "reopened".
	moved = c.expect(t, c.patch("/tasks/"+id+"/move",
		map[string]any{"column_id": s.col(t, 0)}), http.StatusOK)
	if moved["completed_at"] != nil {
		t.Fatalf("move out of done: completed_at still set: %v", moved)
	}
	if kinds = eventKinds(t, c, id); !hasKind(kinds, "reopened") {
		t.Fatalf("events after reopen move: %v", kinds)
	}
}

// Column CRUD: create appends to the end, rename, move (start/middle/end)
// relative to neighbours, delete cascades to the column's tasks.
func TestColumnCRUD(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	// Create → appended after the rightmost default column.
	col := c.expect(t, c.post("/boards/"+s.Board+"/columns",
		map[string]any{"name": "Новая", "color": "#123456"}), http.StatusCreated)
	newID := col["id"].(string)
	if fpos(t, col) <= fpos(t, s.Columns[3]) {
		t.Fatalf("new column position %v not after last default %v", col["position"], s.Columns[3]["position"])
	}

	// Rename (full-replace of name+color).
	col = c.expect(t, c.patch("/columns/"+newID,
		map[string]any{"name": "Переименована", "color": "#654321"}), http.StatusOK)
	if col["name"] != "Переименована" || col["color"] != "#654321" {
		t.Fatalf("column update: %v", col)
	}

	// Move to the very start (only after_id = current first column).
	moved := c.expect(t, c.patch("/columns/"+newID+"/move",
		map[string]any{"after_id": s.col(t, 0)}), http.StatusOK)
	if fpos(t, moved) >= fpos(t, s.Columns[0]) {
		t.Fatalf("move to start: pos %v not before %v", moved["position"], s.Columns[0]["position"])
	}

	// Move to the middle (between columns 0 and 1).
	moved = c.expect(t, c.patch("/columns/"+newID+"/move",
		map[string]any{"before_id": s.col(t, 0), "after_id": s.col(t, 1)}), http.StatusOK)
	if p := fpos(t, moved); p <= fpos(t, s.Columns[0]) || p >= fpos(t, s.Columns[1]) {
		t.Fatalf("move to middle: pos %v not between %v and %v",
			p, s.Columns[0]["position"], s.Columns[1]["position"])
	}

	// Move to the end (only before_id = last column).
	moved = c.expect(t, c.patch("/columns/"+newID+"/move",
		map[string]any{"before_id": s.col(t, 3)}), http.StatusOK)
	if fpos(t, moved) <= fpos(t, s.Columns[3]) {
		t.Fatalf("move to end: pos %v not after %v", moved["position"], s.Columns[3]["position"])
	}

	// The list endpoint reflects the new ordering (moved column last).
	cols := c.get("/boards/" + s.Board + "/columns").listBody(t)
	if len(cols) != 5 || cols[4]["id"] != newID {
		t.Fatalf("column list after moves: %v", cols)
	}

	// A bogus neighbour id → 400.
	if r := c.patch("/columns/"+newID+"/move",
		map[string]any{"before_id": "00000000-0000-0000-0000-000000000001"}); r.Status != http.StatusBadRequest {
		t.Fatalf("invalid before_id: %d", r.Status)
	}

	// Delete removes the column AND its tasks (FK cascade).
	task := mkTask(t, c, s.Board, newID, "Пропадёт с колонкой")
	if r := c.del("/columns/" + newID); r.Status != http.StatusNoContent {
		t.Fatalf("delete column: %d\n%s", r.Status, r.Body)
	}
	if r := c.get("/tasks/" + task["id"].(string)); r.Status != http.StatusNotFound {
		t.Fatalf("task survived column delete: %d", r.Status)
	}
	if cols = c.get("/boards/" + s.Board + "/columns").listBody(t); len(cols) != 4 {
		t.Fatalf("columns after delete = %d, want 4", len(cols))
	}
}

// Saved board views: upsert-by-name semantics, list, delete.
func TestBoardViews(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	// Save (note: returns 200, not 201 — it's an upsert).
	v := c.expect(t, c.post("/boards/"+s.Board+"/views", map[string]any{
		"name": "Мой вид", "config": map[string]any{"cardSize": "small", "stacked": true},
	}), http.StatusOK)
	viewID := v["id"].(string)
	cfg, _ := v["config"].(map[string]any)
	if cfg["cardSize"] != "small" || cfg["stacked"] != true {
		t.Fatalf("view config round-trip: %v", v)
	}

	// Same name overwrites in place (still one view, same id, new config).
	v2 := c.expect(t, c.post("/boards/"+s.Board+"/views", map[string]any{
		"name": "Мой вид", "config": map[string]any{"cardSize": "large"},
	}), http.StatusOK)
	if v2["id"] != viewID {
		t.Fatalf("same-name save created a new view: %v vs %v", v2["id"], viewID)
	}
	list := c.get("/boards/" + s.Board + "/views").listBody(t)
	if len(list) != 1 {
		t.Fatalf("views after upsert = %d, want 1", len(list))
	}
	if cfg, _ = list[0]["config"].(map[string]any); cfg["cardSize"] != "large" {
		t.Fatalf("upsert did not replace config: %v", list[0])
	}

	// Missing config defaults to {}.
	v3 := c.expect(t, c.post("/boards/"+s.Board+"/views", map[string]any{"name": "Пустой"}), http.StatusOK)
	if _, ok := v3["config"].(map[string]any); !ok {
		t.Fatalf("empty config not an object: %v", v3["config"])
	}

	// Delete → 204, gone from the list.
	if r := c.del("/views/" + viewID); r.Status != http.StatusNoContent {
		t.Fatalf("delete view: %d\n%s", r.Status, r.Body)
	}
	list = c.get("/boards/" + s.Board + "/views").listBody(t)
	if hasID(list, viewID) {
		t.Fatalf("deleted view still listed: %v", list)
	}
}

// Board dependency graph: only blocking edges between tasks of the board.
func TestBoardDependencies(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	t1 := mkTask(t, c, s.Board, s.col(t, 0), "Блокирует")
	t2 := mkTask(t, c, s.Board, s.col(t, 0), "Блокируется")

	// Empty graph initially.
	if deps := c.get("/boards/" + s.Board + "/dependencies").listBody(t); len(deps) != 0 {
		t.Fatalf("initial dependencies: %v", deps)
	}

	// t1 blocks t2 (relations are addressed by workspace task number).
	if r := c.post("/tasks/"+t1["id"].(string)+"/relations",
		map[string]any{"number": t2["number"], "kind": "blocks"}); r.Status != http.StatusCreated {
		t.Fatalf("add blocks relation: %d\n%s", r.Status, r.Body)
	}
	// A non-blocking relation must NOT appear in the graph.
	if r := c.post("/tasks/"+t1["id"].(string)+"/relations",
		map[string]any{"number": t2["number"], "kind": "relates"}); r.Status != http.StatusCreated {
		t.Fatalf("add relates relation: %d\n%s", r.Status, r.Body)
	}

	deps := c.get("/boards/" + s.Board + "/dependencies").listBody(t)
	if len(deps) != 1 {
		t.Fatalf("dependencies = %d, want 1 (blocks only): %v", len(deps), deps)
	}
	d := deps[0]
	if d["task_id"] != t1["id"] || d["related_task_id"] != t2["id"] || d["kind"] != "blocks" {
		t.Fatalf("dependency edge: %v", d)
	}
	if !strings.EqualFold(d["kind"].(string), "blocks") {
		t.Fatalf("kind: %v", d["kind"])
	}
}
