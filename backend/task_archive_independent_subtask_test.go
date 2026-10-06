package main

import (
	"net/http"
	"testing"
)

// TestRestoreKeepsIndependentlyArchivedSubtask pins that restoring a parent
// gives back exactly what its archive took — and nothing else.
//
// A subtask can be archived on its own: the archive view lists it as an entry of
// its own precisely because its parent is still active (ListBoardArchivedWithMeta
// hides only the children buried under an archived root, "they restore with it").
// Archive the parent afterwards and that entry disappears from the list — the
// root is the only thing the archive shows. Restoring the root then walked the
// WHOLE subtree, so the deliberately archived subtask came back to the board
// together with the ones the cascade had taken: an undo of a gesture nobody made,
// with no trace anywhere. The archive is also where the user would go looking for
// it, and it is no longer there.
//
// The cascade stamps one archived_at across the subtree (a single UPDATE, one
// transaction timestamp), so "archived with this root" is exactly "carries the
// root's archived_at" — a subtask archived in an earlier request cannot.
func TestRestoreKeepsIndependentlyArchivedSubtask(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	parent := mkTask(t, c, s.Board, col, "родитель")["id"].(string)
	solo := mkTask(t, c, s.Board, col, "убрана сама по себе")["id"].(string)
	withParent := mkTask(t, c, s.Board, col, "уходит вместе с родителем")["id"].(string)
	for _, id := range []string{solo, withParent} {
		c.expect(t, c.patch("/tasks/"+id+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)
	}

	archived := func(t *testing.T) map[string]bool {
		t.Helper()
		seen := map[string]bool{}
		for _, r := range c.get("/boards/" + s.Board + "/archive").listBody(t) {
			seen[r["id"].(string)] = true
		}
		return seen
	}
	isArchived := func(t *testing.T, id string) bool {
		t.Helper()
		return c.expect(t, c.get("/tasks/"+id), http.StatusOK)["archived_at"] != nil
	}

	// The subtask goes to the archive on its own, while its parent stays on the
	// board — the state the archive list has its own branch for.
	c.expect(t, c.patch("/tasks/"+solo+"/archive", map[string]any{}), http.StatusNoContent)

	t.Run("a subtask archived on its own is an archive entry", func(t *testing.T) {
		if seen := archived(t); !seen[solo] {
			t.Fatalf("archive = %v, want the individually archived subtask %s", seen, solo)
		}
	})

	c.expect(t, c.patch("/tasks/"+parent+"/archive", map[string]any{}), http.StatusNoContent)

	t.Run("archiving the parent hides both children behind the root", func(t *testing.T) {
		seen := archived(t)
		if !seen[parent] {
			t.Fatalf("archive = %v, want the archived root %s", seen, parent)
		}
		if seen[solo] || seen[withParent] {
			t.Fatalf("archive = %v, want children hidden under their root", seen)
		}
	})

	c.expect(t, c.patch("/tasks/"+parent+"/restore", map[string]any{}), http.StatusNoContent)

	t.Run("the subtask archived with the parent comes back", func(t *testing.T) {
		for _, id := range []string{parent, withParent} {
			if isArchived(t, id) {
				t.Fatalf("task %s is still archived after its root was restored", id)
			}
		}
	})

	t.Run("the subtask archived on its own stays archived", func(t *testing.T) {
		if !isArchived(t, solo) {
			t.Fatalf("subtask %s was archived on its own before the parent, "+
				"restoring the parent must not put it back on the board", solo)
		}
	})

	t.Run("and is an archive entry again", func(t *testing.T) {
		if seen := archived(t); !seen[solo] {
			t.Fatalf("archive = %v, want %s listed again once its parent is active", seen, solo)
		}
	})
}
