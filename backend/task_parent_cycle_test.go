package main

import (
	"net/http"
	"testing"
)

// TestTaskCannotBecomeItsOwnDescendant pins the re-parent guard on the subtask
// tree, the third self-referencing tree in the schema after documents and
// project groups.
//
// applyParent used to reject only the literal self-parent and the one-level
// flip (the named parent being a direct child of the moved task), which leaves
// the two-step version open: nesting is not depth-limited anywhere — CreateTask
// happily hangs a subtask off a subtask — so build A → B → C and move A under C.
// The table then holds A.parent=C, C.parent=B, B.parent=A: a ring with no root,
// and tasks has neither a constraint nor a trigger to catch it.
//
// The clients survive it — countWithSubtasks (web and Android) carries a `seen`
// set — but the ring still costs its members the board: ListBoardTasks takes
// only parent_id IS NULL, and a subtask is drawn under its parent, so a card
// nobody can reach from a root is drawn nowhere. The cards stay in the database
// and remain findable by search, which is what keeps this a disappearance rather
// than a hang.
func TestTaskCannotBecomeItsOwnDescendant(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	top := mkTask(t, c, s.Board, col, "верхняя")["id"].(string)
	mid := mkTask(t, c, s.Board, col, "средняя")["id"].(string)
	deep := mkTask(t, c, s.Board, col, "глубокая")["id"].(string)

	// top → mid → deep
	c.expect(t, c.patch("/tasks/"+mid+"/parent", map[string]any{"parent_id": top}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+deep+"/parent", map[string]any{"parent_id": mid}), http.StatusOK)

	t.Run("under its own direct child is refused", func(t *testing.T) {
		r := c.patch("/tasks/"+top+"/parent", map[string]any{"parent_id": mid})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("attach under own child: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("under a deeper descendant is refused", func(t *testing.T) {
		r := c.patch("/tasks/"+top+"/parent", map[string]any{"parent_id": deep})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("attach under own grandchild: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("under itself is refused", func(t *testing.T) {
		r := c.patch("/tasks/"+top+"/parent", map[string]any{"parent_id": top})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("attach under itself: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("the chain keeps a single root", func(t *testing.T) {
		// The root is what the board draws from: if the guard let the ring in,
		// top would have a parent and all three would vanish from the lane.
		if got := c.get("/tasks/" + top).mapBody(t); got["parent_id"] != nil {
			t.Fatalf("top task parent_id = %v, want nil (root)", got["parent_id"])
		}
		roots := c.get("/boards/" + s.Board + "/tasks").listBody(t)
		found := false
		for _, r := range roots {
			if r["id"] == top {
				found = true
			}
		}
		if !found {
			t.Fatalf("top task is not among the board's %d top-level cards", len(roots))
		}
	})

	t.Run("a legitimate move still works", func(t *testing.T) {
		// Detaching the tail and re-attaching the middle below it: re-parenting
		// up and down both stay allowed, only the ring is refused.
		c.expect(t, c.patch("/tasks/"+deep+"/parent", map[string]any{"parent_id": nil}), http.StatusOK)
		got := c.expect(t, c.patch("/tasks/"+mid+"/parent", map[string]any{"parent_id": deep}), http.StatusOK)
		if got["parent_id"] != deep {
			t.Fatalf("mid task parent_id = %v, want %s", got["parent_id"], deep)
		}
	})
}
