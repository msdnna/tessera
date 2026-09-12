package main

import (
	"net/http"
	"testing"
)

// TestTaskTransferCarriesWholeSubtree pins that a board change takes the task's
// entire descendant tree with it, not just its direct children.
//
// TransferTask moved `WHERE parent_id = $1` — one level — while nesting is not
// depth-limited anywhere (CreateTask hangs a subtask off a subtask, and the
// re-parent guard next door exists precisely because chains run deeper than
// one). So a grandchild kept the board it was created on while its parent left,
// and that is a disappearance: the kanban draws a subtask only under a parent
// card present on the same board (subtasksByParent), the old board no longer
// holds the parent, and the new board's subtask query filters by board_id, so
// the row is fetched by neither.
//
// It is also destructible. tasks.board_id and tasks.column_id are ON DELETE
// CASCADE, so deleting the column the grandchild was left behind in — a column
// on a board its subtree no longer has anything to do with — silently deletes
// the task itself.
func TestTaskTransferCarriesWholeSubtree(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)
	board2 := mkBoard(t, c, s.Project, "Целевая доска")
	cols2 := c.get("/boards/" + board2 + "/columns").listBody(t)
	target := cols2[0]["id"].(string)

	// top → mid → deep, all three on the source board.
	top := mkTask(t, c, s.Board, col, "верхняя")["id"].(string)
	mid := mkTask(t, c, s.Board, col, "средняя")["id"].(string)
	deep := mkTask(t, c, s.Board, col, "глубокая")["id"].(string)
	c.expect(t, c.patch("/tasks/"+mid+"/parent", map[string]any{"parent_id": top}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+deep+"/parent", map[string]any{"parent_id": mid}), http.StatusOK)

	c.expect(t, c.patch("/tasks/"+top+"/transfer", map[string]any{
		"board_id": board2, "column_id": target,
	}), http.StatusOK)

	t.Run("every descendant lands on the target board", func(t *testing.T) {
		for _, id := range []string{mid, deep} {
			got := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
			if got["board_id"] != board2 || got["column_id"] != target {
				t.Fatalf("task %s after transfer: board=%v column=%v, want %s / %s",
					id, got["board_id"], got["column_id"], board2, target)
			}
		}
	})

	t.Run("the target board draws them under their parents", func(t *testing.T) {
		subs := c.get("/boards/" + board2 + "/subtasks").listBody(t)
		seen := map[string]bool{}
		for _, r := range subs {
			seen[r["id"].(string)] = true
		}
		if !seen[mid] || !seen[deep] {
			t.Fatalf("target board subtasks = %v, want both %s and %s", seen, mid, deep)
		}
	})

	t.Run("the source board keeps none of them", func(t *testing.T) {
		for _, r := range c.get("/boards/" + s.Board + "/subtasks").listBody(t) {
			if r["id"] == mid || r["id"] == deep {
				t.Fatalf("task %v is still a subtask of the source board", r["id"])
			}
		}
	})
}

// TestReparentAcrossBoardsCarriesSubtree is the same rule for the other door
// onto board_id. applyParent adopts the new parent's board and column for the
// moved task alone, so attaching a task that has children under a parent on
// another board stranded the children on the board the task just left — the
// one-level failure above, but starting at the first level rather than the
// second.
func TestReparentAcrossBoardsCarriesSubtree(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)
	board2 := mkBoard(t, c, s.Project, "Доска нового родителя")
	cols2 := c.get("/boards/" + board2 + "/columns").listBody(t)
	host := mkTask(t, c, board2, cols2[0]["id"].(string), "принимающая")["id"].(string)

	// mover → child, both on the source board.
	mover := mkTask(t, c, s.Board, col, "переезжает")["id"].(string)
	child := mkTask(t, c, s.Board, col, "ребёнок")["id"].(string)
	c.expect(t, c.patch("/tasks/"+child+"/parent", map[string]any{"parent_id": mover}), http.StatusOK)

	moved := c.expect(t, c.patch("/tasks/"+mover+"/parent", map[string]any{"parent_id": host}), http.StatusOK)
	if moved["board_id"] != board2 {
		t.Fatalf("moved task board = %v, want %s", moved["board_id"], board2)
	}

	got := c.expect(t, c.get("/tasks/"+child), http.StatusOK)
	if got["board_id"] != moved["board_id"] || got["column_id"] != moved["column_id"] {
		t.Fatalf("child after re-parent: board=%v column=%v, want %v / %v",
			got["board_id"], got["column_id"], moved["board_id"], moved["column_id"])
	}
}
