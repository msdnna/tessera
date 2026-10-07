// Soft delete of boards and columns (migration 0078, #2969 / #3002 / #3003).
//
// The v2 client deletes a board or a column at once and offers the undo in a
// toast. That only works if the server keeps what it hid: a deleted board must
// vanish for every reader yet come back whole, and a deleted column must hand
// its cards to a neighbour and take them back on restore.
package main

import (
	"context"
	"net/http"
	"testing"
)

func TestDeletedBoardVanishesAndRestores(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	task := mkTask(t, owner, s.Board, s.col(t, 1), "Задача удалённой доски")
	taskID := task["id"].(string)
	number := int(task["number"].(float64))

	owner.expect(t, owner.del("/boards/"+s.Board), http.StatusNoContent)

	// Gone for every reader: the board itself, its task, the project's board
	// list, the workspace task list, the by-number link and search.
	for _, path := range []string{
		"/boards/" + s.Board,
		"/tasks/" + taskID,
		"/boards/" + s.Board + "/tasks",
		"/workspaces/" + s.WS + "/tasks/by-number/" + itoa(number),
	} {
		if r := owner.get(path); r.Status != http.StatusNotFound {
			t.Fatalf("GET %s on a deleted board: %d, want 404\n%s", path, r.Status, r.Body)
		}
	}
	for _, b := range owner.get("/projects/" + s.Project + "/boards").listBody(t) {
		if b["id"] == s.Board {
			t.Fatal("deleted board still listed in its project")
		}
	}
	for _, wt := range owner.get("/workspaces/" + s.WS + "/tasks").listBody(t) {
		if wt["id"] == taskID {
			t.Fatal("task of a deleted board still in the workspace task list")
		}
	}

	// A second delete finds nothing to delete.
	if r := owner.del("/boards/" + s.Board); r.Status != http.StatusNotFound {
		t.Fatalf("second delete: %d, want 404", r.Status)
	}

	restored := owner.expect(t, owner.post("/boards/"+s.Board+"/restore", nil), http.StatusOK)
	if restored["id"] != s.Board || restored["deleted_at"] != nil {
		t.Fatalf("restore answered %v", restored)
	}
	got := owner.expect(t, owner.get("/tasks/"+taskID), http.StatusOK)
	if got["column_id"] != s.col(t, 1) {
		t.Fatalf("restored task in column %v, want %s", got["column_id"], s.col(t, 1))
	}
	// Restoring a live board is not a thing.
	if r := owner.post("/boards/"+s.Board+"/restore", nil); r.Status != http.StatusNotFound {
		t.Fatalf("restore of a live board: %d, want 404", r.Status)
	}
}

func TestRestoreBoardNeedsMembership(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	stranger := signup(t)
	s := mkStack(t, owner)
	owner.expect(t, owner.del("/boards/"+s.Board), http.StatusNoContent)
	if r := stranger.post("/boards/"+s.Board+"/restore", nil); r.Status != http.StatusForbidden && r.Status != http.StatusNotFound {
		t.Fatalf("stranger restore: %d, want 403/404", r.Status)
	}
	owner.expect(t, owner.post("/boards/"+s.Board+"/restore", nil), http.StatusOK)
}

func TestDeletedColumnHandsCardsOverAndTakesThemBack(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	doomed, first := s.col(t, 1), s.col(t, 0)

	a := mkTask(t, owner, s.Board, doomed, "Первая")["id"].(string)
	b := mkTask(t, owner, s.Board, doomed, "Вторая")["id"].(string)
	stay := mkTask(t, owner, s.Board, first, "Уже была в первой")["id"].(string)
	posA := owner.expect(t, owner.get("/tasks/"+a), http.StatusOK)["position"]

	owner.expect(t, owner.del("/columns/"+doomed), http.StatusNoContent)

	for _, c := range owner.get("/boards/" + s.Board + "/columns").listBody(t) {
		if c["id"] == doomed {
			t.Fatal("deleted column still listed")
		}
	}
	// Moved cards land after what the target already held, in their own order.
	order := map[string]float64{}
	for _, id := range []string{stay, a, b} {
		got := owner.expect(t, owner.get("/tasks/"+id), http.StatusOK)
		if got["column_id"] != first {
			t.Fatalf("task %s in %v, want the first column", id, got["column_id"])
		}
		order[id] = got["position"].(float64)
	}
	if !(order[stay] < order[a] && order[a] < order[b]) {
		t.Fatalf("moved cards out of order: %v", order)
	}
	// Nothing can be moved into the deleted column.
	if r := owner.patch("/tasks/"+stay+"/move", map[string]any{"column_id": doomed}); r.Status == http.StatusOK {
		t.Fatal("moved a card into a deleted column")
	}

	// Someone drags b elsewhere before the undo: the restore leaves it there.
	owner.expect(t, owner.patch("/tasks/"+b+"/move", map[string]any{"column_id": s.col(t, 2)}), http.StatusOK)

	restored := owner.expect(t, owner.post("/columns/"+doomed+"/restore", nil), http.StatusOK)
	if restored["id"] != doomed {
		t.Fatalf("restore answered %v", restored)
	}
	gotA := owner.expect(t, owner.get("/tasks/"+a), http.StatusOK)
	if gotA["column_id"] != doomed || gotA["position"] != posA {
		t.Fatalf("restored card: column %v position %v, want %s / %v", gotA["column_id"], gotA["position"], doomed, posA)
	}
	if got := owner.expect(t, owner.get("/tasks/"+b), http.StatusOK); got["column_id"] != s.col(t, 2) {
		t.Fatalf("a card moved after the delete was pulled back: %v", got["column_id"])
	}
	if got := owner.expect(t, owner.get("/tasks/"+stay), http.StatusOK); got["column_id"] != first {
		t.Fatalf("a card that was never in the column moved: %v", got["column_id"])
	}
}

// Deleting the completing column sends its (finished) cards to the column that
// takes over the done flag, still completed; the restore gives the flag back.
func TestDeletedDoneColumnKeepsCardsCompleted(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	done := boardDone(t, owner, s.Board)
	heir := s.col(t, len(s.Columns)-2)

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Сделано")["id"].(string)
	owner.expect(t, owner.patch("/tasks/"+task+"/move", map[string]any{"column_id": done}), http.StatusOK)

	owner.expect(t, owner.del("/columns/"+done), http.StatusNoContent)
	if got := boardDone(t, owner, s.Board); got != heir {
		t.Fatalf("done flag on %q, want the new rightmost %s", got, heir)
	}
	got := owner.expect(t, owner.get("/tasks/"+task), http.StatusOK)
	if got["column_id"] != heir || got["completed_at"] == nil {
		t.Fatalf("finished card: column %v completed %v, want %s and completed", got["column_id"], got["completed_at"], heir)
	}

	owner.expect(t, owner.post("/columns/"+done+"/restore", nil), http.StatusOK)
	if got := boardDone(t, owner, s.Board); got != done {
		t.Fatalf("done flag on %q after restore, want %s back", got, done)
	}
	if got := owner.expect(t, owner.get("/tasks/"+task), http.StatusOK); got["column_id"] != done {
		t.Fatalf("finished card in %v after restore, want %s", got["column_id"], done)
	}
}

// The trash worker finishes a column delete once its undo window is over; until
// then a restore still works, after it there is nothing left to restore.
func TestPurgedColumnCannotBeRestored(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	doomed := s.col(t, 1)
	owner.expect(t, owner.del("/columns/"+doomed), http.StatusNoContent)

	ageTrash(t, "board_columns", doomed)
	testAPI.PurgeTrash(context.Background())

	if r := owner.post("/columns/"+doomed+"/restore", nil); r.Status != http.StatusNotFound {
		t.Fatalf("restore after purge: %d, want 404", r.Status)
	}
}
