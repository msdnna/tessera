package main

import (
	"net/http"
	"testing"
)

// Attaching a task to a parent moves the card: applyParent adopts the parent's
// board AND column, so a finished card pulled under a parent that stands in «К
// работе» физически leaves the done column. The board's rule for that gesture is
// the one applyMove states — what leaves a done column for a column that does
// not complete is reopened — and /transfer, the other door onto the same field,
// already follows it (doneColumnLeftBy + ReopenSubtreeLeavingDone).
//
// The re-parent did not. The card landed struck through in a column that
// completes nothing, counted as done, and out of reach of the reopen rule: it
// fires only for a task standing in a done column, and this one no longer is.
// The only way back was to drag it into «Готово» and out again.
func TestReparentOutOfDoneReopens(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	backlog, done := s.col(t, 0), s.col(t, 3)

	parent := mkTask(t, c, s.Board, backlog, "родитель в работе")["id"].(string)
	child := mkTask(t, c, s.Board, backlog, "выполненная")["id"].(string)

	// Finish it the ordinary way — by dragging it into the done column.
	finished := c.expect(t, c.patch("/tasks/"+child+"/move",
		map[string]any{"column_id": done}), http.StatusOK)
	if finished["completed_at"] == nil {
		t.Fatalf("fixture: move into done left completed_at nil: %v", finished)
	}

	got := c.expect(t, c.patch("/tasks/"+child+"/parent",
		map[string]any{"parent_id": parent}), http.StatusOK)

	t.Run("the card follows the parent out of the done column", func(t *testing.T) {
		if got["column_id"] != backlog {
			t.Fatalf("column after re-parent = %v, want %s", got["column_id"], backlog)
		}
	})

	t.Run("and is no longer completed", func(t *testing.T) {
		if got["completed_at"] != nil {
			t.Fatalf("completed_at after re-parent = %v, want nil", got["completed_at"])
		}
		// The response must not be the only place it shows: a second reader sees
		// the task through GET, not through the answer to someone else's PATCH.
		if reread := c.expect(t, c.get("/tasks/"+child), http.StatusOK); reread["completed_at"] != nil {
			t.Fatalf("re-read completed_at = %v, want nil", reread["completed_at"])
		}
	})

	t.Run("the journal says so, as an in-board reopen would", func(t *testing.T) {
		if kinds := eventKinds(t, c, child); !hasKind(kinds, "reopened") {
			t.Fatalf("events after re-parent = %v, want a reopened entry", kinds)
		}
	})
}

// The mirror image: a parent standing in the done column itself takes nothing
// away — the card moves from one done column to the same one, which is not a
// departure. (Nor does it complete anything: only applyMove completes, and this
// pins that the reopen pass does not invent the opposite rule either.)
func TestReparentWithinDoneKeepsCompletion(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	done := s.col(t, 3)

	parent := mkTask(t, c, s.Board, done, "родитель в готово")["id"].(string)
	child := mkTask(t, c, s.Board, s.col(t, 0), "выполненная")["id"].(string)
	c.expect(t, c.patch("/tasks/"+child+"/move", map[string]any{"column_id": done}), http.StatusOK)

	got := c.expect(t, c.patch("/tasks/"+child+"/parent",
		map[string]any{"parent_id": parent}), http.StatusOK)
	if got["completed_at"] == nil {
		t.Fatalf("re-parent inside the done column cleared completed_at: %v", got)
	}
}

// A task completed while standing anywhere else keeps its flag — the condition
// is the column being left, not the flag alone. A board that does not tie
// completion to columns must not have that tie invented for it by a re-parent
// (the same sentence doneColumnLeftBy already writes for /transfer).
func TestReparentKeepsCompletionSetOutsideDoneColumn(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	backlog, middle := s.col(t, 0), s.col(t, 1)

	// No done column on this board at all.
	c.expect(t, c.patch("/boards/"+s.Board+"/done-column",
		map[string]any{"column_id": nil}), http.StatusOK)

	parent := mkTask(t, c, s.Board, backlog, "родитель")["id"].(string)
	child := mkTask(t, c, s.Board, middle, "отмечена вручную")["id"].(string)
	marked := c.expect(t, c.patch("/tasks/"+child, map[string]any{"completed": true}), http.StatusOK)
	if marked["completed_at"] == nil {
		t.Fatalf("fixture: PATCH completed=true left completed_at nil: %v", marked)
	}

	got := c.expect(t, c.patch("/tasks/"+child+"/parent",
		map[string]any{"parent_id": parent}), http.StatusOK)
	if got["completed_at"] == nil {
		t.Fatalf("re-parent cleared a completion the board's columns never set: %v", got)
	}
}

// Detaching changes no column (applyParent keeps the task's own board/column
// when parent_id is null), so a finished subtask standing in the done column
// stays finished when it is lifted back to the top level.
func TestDetachKeepsCompletion(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	done := s.col(t, 3)

	parent := mkTask(t, c, s.Board, done, "родитель")["id"].(string)
	child := mkTask(t, c, s.Board, s.col(t, 0), "подзадача")["id"].(string)
	c.expect(t, c.patch("/tasks/"+child+"/parent", map[string]any{"parent_id": parent}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+child+"/move", map[string]any{"column_id": done}), http.StatusOK)

	got := c.expect(t, c.patch("/tasks/"+child+"/parent",
		map[string]any{"parent_id": nil}), http.StatusOK)
	if got["completed_at"] == nil {
		t.Fatalf("detach cleared completed_at without moving the card: %v", got)
	}
}

// Within one board the descendants do not move (applyParent only re-boards a
// subtree when the board itself changes), so a finished grandchild sitting in
// the done column must be left exactly where it is — the reopen pass follows
// the cards that actually left the column, not the whole subtree.
func TestReparentWithinBoardLeavesDescendantsCompleted(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	backlog, done := s.col(t, 0), s.col(t, 3)

	parent := mkTask(t, c, s.Board, backlog, "новый родитель")["id"].(string)
	mover := mkTask(t, c, s.Board, backlog, "переезжает")["id"].(string)
	kid := mkTask(t, c, s.Board, backlog, "ребёнок в готово")["id"].(string)
	c.expect(t, c.patch("/tasks/"+kid+"/parent", map[string]any{"parent_id": mover}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+kid+"/move", map[string]any{"column_id": done}), http.StatusOK)

	// The mover itself is not completed and is not in the done column.
	c.expect(t, c.patch("/tasks/"+mover+"/parent",
		map[string]any{"parent_id": parent}), http.StatusOK)

	got := c.expect(t, c.get("/tasks/"+kid), http.StatusOK)
	if got["completed_at"] == nil {
		t.Fatalf("child that never moved was reopened: %v", got)
	}
	if got["column_id"] != done {
		t.Fatalf("child column = %v, want %s (same-board re-parent must not move it)", got["column_id"], done)
	}
}

// Across boards the subtree DOES travel (MoveSubtreeToBoard), landing in the new
// parent's column — so every finished descendant leaves its done column too, and
// the transfer's answer applies to all of them, not just the root.
func TestReparentAcrossBoardsReopensWholeSubtree(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	done := s.col(t, 3)
	board2 := mkBoard(t, c, s.Project, "Доска нового родителя")
	cols2 := c.get("/boards/" + board2 + "/columns").listBody(t)
	host := mkTask(t, c, board2, cols2[0]["id"].(string), "принимающая")["id"].(string)

	mover := mkTask(t, c, s.Board, s.col(t, 0), "переезжает")["id"].(string)
	kid := mkTask(t, c, s.Board, s.col(t, 0), "ребёнок")["id"].(string)
	c.expect(t, c.patch("/tasks/"+kid+"/parent", map[string]any{"parent_id": mover}), http.StatusOK)
	// Both finished in the source board's done column.
	c.expect(t, c.patch("/tasks/"+kid+"/move", map[string]any{"column_id": done}), http.StatusOK)
	c.expect(t, c.patch("/tasks/"+mover+"/move", map[string]any{"column_id": done}), http.StatusOK)

	c.expect(t, c.patch("/tasks/"+mover+"/parent",
		map[string]any{"parent_id": host}), http.StatusOK)

	for _, id := range []string{mover, kid} {
		got := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
		if got["completed_at"] != nil {
			t.Fatalf("task %s kept completed_at after leaving the source done column: %v", id, got["completed_at"])
		}
		if got["board_id"] != board2 {
			t.Fatalf("task %s board = %v, want %s", id, got["board_id"], board2)
		}
		if kinds := eventKinds(t, c, id); !hasKind(kinds, "reopened") {
			t.Fatalf("task %s events = %v, want a reopened entry", id, kinds)
		}
	}
}
