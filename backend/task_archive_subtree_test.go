package main

import (
	"net/http"
	"testing"
)

// TestArchiveCarriesWholeSubtree pins that archiving a task takes its entire
// descendant tree with it — and gives all of it back on restore.
//
// ArchiveTaskCascade/RestoreTask moved `WHERE id = $1 OR parent_id = $1` — one
// level — while nesting is not depth-limited (CreateTask hangs a subtask off a
// subtask, and MoveSubtreeToBoard next door is recursive for exactly that
// reason). A grandchild therefore stayed active under an archived parent, which
// is a disappearance from both views at once: the kanban draws a subtask only
// under a parent card present in the same payload (subtasksByParent) and the
// parent is archived, while the archive list filters on archived_at IS NOT NULL
// and the grandchild's is still NULL. The row is fetched by neither screen.
func TestArchiveCarriesWholeSubtree(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	// top → mid → deep.
	top := mkTask(t, c, s.Board, col, "верхняя")["id"].(string)
	mid := mkTask(t, c, s.Board, col, "средняя")["id"].(string)
	deep := mkTask(t, c, s.Board, col, "глубокая")["id"].(string)
	c.expect(t, c.patch("/tasks/"+mid+"/parent", map[string]any{"parent_id": top}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+deep+"/parent", map[string]any{"parent_id": mid}), http.StatusOK)

	c.expect(t, c.patch("/tasks/"+top+"/archive", map[string]any{}), http.StatusNoContent)

	t.Run("every descendant is archived with the parent", func(t *testing.T) {
		for _, id := range []string{mid, deep} {
			got := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
			if got["archived_at"] == nil {
				t.Fatalf("task %s is still active after its ancestor was archived", id)
			}
		}
	})

	t.Run("the active board no longer draws them", func(t *testing.T) {
		for _, r := range c.get("/boards/" + s.Board + "/subtasks").listBody(t) {
			if r["id"] == mid || r["id"] == deep {
				t.Fatalf("archived task %v is still on the active board", r["id"])
			}
		}
	})

	t.Run("the archive lists the root once, not its buried children", func(t *testing.T) {
		seen := map[string]bool{}
		for _, r := range c.get("/boards/" + s.Board + "/archive").listBody(t) {
			seen[r["id"].(string)] = true
		}
		if !seen[top] {
			t.Fatalf("archive = %v, want the archived root %s", seen, top)
		}
		if seen[mid] || seen[deep] {
			t.Fatalf("archive = %v, want children hidden under their root", seen)
		}
	})

	t.Run("restore gives the whole tree back", func(t *testing.T) {
		c.expect(t, c.patch("/tasks/"+top+"/restore", map[string]any{}), http.StatusNoContent)
		for _, id := range []string{top, mid, deep} {
			got := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
			if got["archived_at"] != nil {
				t.Fatalf("task %s is still archived after the root was restored", id)
			}
		}
	})
}
