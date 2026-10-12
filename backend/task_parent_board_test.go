package main

import (
	"net/http"
	"testing"
)

// TestSubtaskCannotBeCreatedOnAnotherBoard pins the one invariant the whole
// subtask rendering rests on: a subtask stands on its parent's board.
//
// Every other door already enforces it. applyParent makes the task ADOPT the
// parent's board and carries its subtree along (task_ops.go), /transfer clears
// parent_id and moves the subtree with MoveSubtreeToBoard, and the MCP
// create_task overwrites the caller's board_id with parent.BoardID. Only
// POST /boards/:id/tasks did not: it validated the named parent against the
// WORKSPACE (so a stranger's task cannot be adopted) and then wrote the URL's
// board next to it, board mismatch and all.
//
// What that costs is the card. The two board queries split the work by the
// parent's state, not the parent's board:
//   - ListBoardTasksWithMeta serves "top-level for this view", i.e. no parent the
//     same view would also draw — the child has an active parent, so it is NOT a
//     card;
//   - ListBoardSubtasksWithMeta serves board_id = this board AND an active
//     parent — the child passes both, so the board is handed a subtask whose
//     parent card is not in the payload, and KanbanBoard files it under
//     byParent[parent_id] where nothing ever reads it.
//
// So on the child's own board it is neither a card nor a rendered subtask, on the
// parent's board it is not fetched at all (board_id differs), and the archive
// view wants archived_at IS NOT NULL. It survives only in the parent's modal
// (ListSubtasksWithMeta keys on parent_id alone) and in search. Deleting the
// board it was filed on takes it with it — board_id is ON DELETE CASCADE.
func TestSubtaskCannotBeCreatedOnAnotherBoard(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	col := s.col(t, 0)

	// A second board in the SAME project, so the workspace check the handler does
	// perform cannot be what refuses this.
	other := mkBoard(t, c, s.Project, "Вторая доска")
	otherCol := c.get("/boards/" + other + "/columns").listBody(t)[0]["id"].(string)

	parent := mkTask(t, c, s.Board, col, "родитель")["id"].(string)

	t.Run("parent on another board is refused", func(t *testing.T) {
		r := c.post("/boards/"+other+"/tasks", map[string]any{
			"title": "беспризорная", "column_id": otherCol, "parent_id": parent,
		})
		if r.Status != http.StatusBadRequest {
			t.Fatalf("subtask across boards: got %d %s, want 400", r.Status, r.Body)
		}
	})

	t.Run("no orphan is drawn on either board", func(t *testing.T) {
		// The proof the refusal is worth having: had the row been created, it would
		// be absent from all four lists at once.
		for _, probe := range []struct{ label, path string }{
			{"cards of the other board", "/boards/" + other + "/tasks"},
			{"subtasks of the other board", "/boards/" + other + "/subtasks"},
			{"cards of the parent's board", "/boards/" + s.Board + "/tasks"},
			{"subtasks of the parent's board", "/boards/" + s.Board + "/subtasks"},
		} {
			for _, row := range c.get(probe.path).listBody(t) {
				if row["title"] == "беспризорная" {
					t.Fatalf("%s: the orphan subtask is drawn there", probe.label)
				}
			}
		}
	})

	// ── contrast: the shapes that must keep working ──

	t.Run("parent on the same board is accepted", func(t *testing.T) {
		m := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "своя подзадача", "column_id": col, "parent_id": parent,
		}), http.StatusCreated)
		if m["parent_id"] != parent {
			t.Fatalf("parent_id: got %v, want %s", m["parent_id"], parent)
		}
	})

	t.Run("parent on the same board in another column is accepted", func(t *testing.T) {
		// A subtask standing in a different column than its parent is legal
		// (DetachChildren counts on it) — the guard is about boards, not columns.
		m := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "подзадача в другой колонке", "column_id": s.col(t, 1), "parent_id": parent,
		}), http.StatusCreated)
		if m["parent_id"] != parent {
			t.Fatalf("parent_id: got %v, want %s", m["parent_id"], parent)
		}
	})

	t.Run("a top-level card on the other board is accepted", func(t *testing.T) {
		mkTask(t, c, other, otherCol, "обычная карточка")
	})
}
