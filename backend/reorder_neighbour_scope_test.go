package main

import (
	"net/http"
	"testing"
)

// Every orderable list in Tessera — a column's cards, a parent's subtasks, a
// board's columns, a workspace's projects and groups — is numbered on its OWN
// float8 ladder: positions are compared within the list and never across. A
// reorder request names its two neighbours by id, and the server takes the
// midpoint of their positions.
//
// So a neighbour from a different list hands that arithmetic a number with no
// meaning in the destination, and the item lands at an arbitrary slot with no
// error raised anywhere. The ids come from the client and genuinely do go stale:
// both clients freeze the board on purpose for the length of a drag (web
// `suppress()`, Android `dragging`), so a colleague moving the very card you are
// aiming next to reaches you only after you let go. A neighbour that was DELETED
// was always answered honestly (the lookup fails → 400 → the client reloads); a
// neighbour that merely moved elsewhere was the silent one.
//
// The same lookups are also the reason a bare id used to report whether it
// exists outside the caller's own workspaces — they were scoped by nothing but
// the id. All of it answers 400 now, the same one an unknown id gets (#2932).

func TestMoveTaskRejectsNeighbourFromAnotherColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo, doing := s.col(t, 0), s.col(t, 1)

	// Two cards standing in the destination, plus a stranger one column over.
	first := mkTask(t, c, s.Board, doing, "Первая")["id"].(string)
	second := mkTask(t, c, s.Board, doing, "Вторая")["id"].(string)
	stranger := mkTask(t, c, s.Board, todo, "Из другой колонки")["id"].(string)
	mover := mkTask(t, c, s.Board, todo, "Перетаскиваемая")["id"].(string)

	r := c.patch("/tasks/"+mover+"/move", map[string]any{
		"column_id": doing, "before_id": stranger, "after_id": second,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("move with a neighbour from another column: status %d, want 400\n%s", r.Status, r.Body)
	}
	// Refused means refused: the card is still where it was.
	got := c.expect(t, c.get("/tasks/"+mover), http.StatusOK)
	if got["column_id"] != todo {
		t.Fatalf("mover moved anyway: column %v, want %s", got["column_id"], todo)
	}

	// The same drop between two real siblings goes through untouched.
	c.expect(t, c.patch("/tasks/"+mover+"/move", map[string]any{
		"column_id": doing, "before_id": first, "after_id": second,
	}), http.StatusOK)
	// Only the destination column's three: the board's task list is one global
	// `ORDER BY position, …`, and columns number their cards independently, so
	// `stranger` interleaves with them by arithmetic accident.
	ids := idsIn(t, c, "/boards/"+s.Board+"/tasks")
	assertSameOrder(t, "cards after a legit drop", filterIDs(ids, first, mover, second),
		[]string{first, mover, second})
}

func TestMoveTaskRejectsSubtaskAsTopLevelNeighbour(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	todo, doing := s.col(t, 0), s.col(t, 1)

	parent := mkTask(t, c, s.Board, doing, "Родитель")["id"].(string)
	child := mkTask(t, c, s.Board, doing, "Ребёнок")["id"].(string)
	c.expect(t, c.patch("/tasks/"+child+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)
	mover := mkTask(t, c, s.Board, todo, "Перетаскиваемая")["id"].(string)

	// Subtasks are ordered among their parent's children, on a ladder of their
	// own — a top-level drop may not name one as its neighbour even though it
	// stands in the same column.
	r := c.patch("/tasks/"+mover+"/move", map[string]any{
		"column_id": doing, "after_id": child,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("top-level drop next to a subtask: status %d, want 400\n%s", r.Status, r.Body)
	}

	// And the mirror: a subtask reordering among its siblings may not name a
	// top-level card, but may name a real sibling.
	sibling := mkTask(t, c, s.Board, doing, "Второй ребёнок")["id"].(string)
	c.expect(t, c.patch("/tasks/"+sibling+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)
	r = c.patch("/tasks/"+child+"/move", map[string]any{
		"column_id": doing, "after_id": mover,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("subtask next to a top-level card: status %d, want 400\n%s", r.Status, r.Body)
	}
	c.expect(t, c.patch("/tasks/"+child+"/move", map[string]any{
		"column_id": doing, "after_id": sibling,
	}), http.StatusOK)
}

func TestMoveColumnRejectsNeighbourFromAnotherBoard(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	other := mkBoard(t, c, s.Project, "Соседняя доска")
	stranger := c.get("/boards/" + other + "/columns").listBody(t)[0]["id"].(string)

	before := idsIn(t, c, "/boards/"+s.Board+"/columns")
	mover := s.col(t, 2)

	r := c.patch("/columns/"+mover+"/move", map[string]any{"after_id": stranger})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("column move onto another board's ladder: status %d, want 400\n%s", r.Status, r.Body)
	}
	assertSameOrder(t, "columns after a refused move", idsIn(t, c, "/boards/"+s.Board+"/columns"), before)

	// A neighbour on this board still works.
	c.expect(t, c.patch("/columns/"+mover+"/move",
		map[string]any{"after_id": s.col(t, 0)}), http.StatusOK)
	assertSameOrder(t, "columns after a legit move", idsIn(t, c, "/boards/"+s.Board+"/columns"),
		[]string{before[2], before[0], before[1], before[3]})
}

func TestMoveSidebarItemRejectsNeighbourFromAnotherWorkspace(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	// A second workspace the same user owns — so this is about the ladder, not
	// about membership; a non-member is refused a step earlier, by loadProject.
	otherWs := mkWorkspace(t, c, "Соседний WS "+t.Name())
	otherGroup := mkGroup(t, c, otherWs, "Чужая группа")
	otherProject := mkProject(t, c, otherWs, otherGroup, "Чужой проект")

	sibling := mkProject(t, c, s.WS, s.Group, "Сосед по проекту")

	r := c.patch("/projects/"+s.Project+"/move", map[string]any{
		"group_id": s.Group, "after_id": otherProject,
	})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("project move next to another workspace's project: status %d, want 400\n%s", r.Status, r.Body)
	}
	c.expect(t, c.patch("/projects/"+s.Project+"/move", map[string]any{
		"group_id": s.Group, "after_id": sibling,
	}), http.StatusOK)

	ownGroup := mkGroup(t, c, s.WS, "Сосед по группе")
	r = c.patch("/groups/"+s.Group+"/move", map[string]any{"after_id": otherGroup})
	if r.Status != http.StatusBadRequest {
		t.Fatalf("group move next to another workspace's group: status %d, want 400\n%s", r.Status, r.Body)
	}
	c.expect(t, c.patch("/groups/"+s.Group+"/move",
		map[string]any{"after_id": ownGroup}), http.StatusOK)
}

// filterIDs keeps only the wanted ids, in the order `ids` has them — a board's
// task list carries everything the fixture made, and only the relative order of
// a few of them is the point.
func filterIDs(ids []string, want ...string) []string {
	keep := make(map[string]bool, len(want))
	for _, w := range want {
		keep[w] = true
	}
	var out []string
	for _, id := range ids {
		if keep[id] {
			out = append(out, id)
		}
	}
	return out
}
