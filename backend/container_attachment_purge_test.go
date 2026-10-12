package main

import (
	"context"
	"net/http"
	"testing"
)

// What a deleted *container* leaves on disk (#2932).
//
// DeleteTask sweeps <uploadDir>/<taskID>/ for every task the delete really took
// (see task_attachment_purge_test.go). But deleting a task is not the only way to
// delete one: board_columns, boards and projects all own their tasks through
// ON DELETE CASCADE, and a workspace owns them two levels further up. Those four
// handlers issue a single DELETE and answer 204 — the task rows go, the
// task_attachments rows go with them, and storage_path, the only record of where
// the bytes live, goes too. Nothing ever walks the upload root afterwards.
//
// The gesture is not exotic: "удалить доску" / "удалить проект" is how a team
// clears a finished quarter, and it takes far more tasks at once than deleting
// them one by one ever would — 25 MB per file, no cap on the count.
//
// Since the soft delete (0078, #2969) a column and a board no longer delete at
// once: a column hands its cards to a neighbour, a board waits in the trash, and
// the trash worker does the board's sweep when it finishes the delete.
//
// Each test keeps a sibling container alive. A sweep that cleared the upload root
// — the cheapest wrong fix — would satisfy every "the files are gone" assertion
// and fail exactly these.

// TestDeletingAColumnKeepsItsTasksAttachments: since the soft delete (0078,
// #2969) a column no longer takes its cards along — they move to the board's
// first remaining column — so their files must stay exactly where they are.
func TestDeletingAColumnKeepsItsTasksAttachments(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	doomed, first := s.col(t, 0), s.col(t, 1)

	task := mkTask(t, owner, s.Board, doomed, "Карточка в удаляемой колонке")["id"].(string)
	attachFile(t, owner, task, "смета.pdf", []byte("%PDF-1.7 smeta"))

	owner.expect(t, owner.del("/columns/"+doomed), http.StatusNoContent)

	got := owner.expect(t, owner.get("/tasks/"+task), http.StatusOK)
	if got["column_id"] != first {
		t.Fatalf("card of a deleted column landed in %v, want the first remaining column %s", got["column_id"], first)
	}
	if n, ok := countFiles(t, taskUploadDir(task)); !ok || n != 1 {
		t.Fatalf("column delete touched the card's files: %d left (exists=%v)", n, ok)
	}
}

// TestDeletingAColumnLeavesSubtasksStandingElsewhere: a subtask is a card of its
// own and stands wherever it was dragged. Deleting its parent's column moves the
// parent; the subtask stays in its column, its place among its siblings and its
// files untouched.
func TestDeletingAColumnLeavesSubtasksStandingElsewhere(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	doomed, kept := s.col(t, 0), s.col(t, 2)

	parent := mkTask(t, owner, s.Board, doomed, "Эпик в удаляемой колонке")["id"].(string)
	child := owner.expect(t, owner.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Подзадача на ревью", "column_id": doomed, "parent_id": parent,
	}), http.StatusCreated)["id"].(string)
	owner.expect(t, owner.patch("/tasks/"+child+"/move",
		map[string]any{"column_id": kept}), http.StatusOK)
	attachFile(t, owner, child, "патч.diff", []byte("--- a\n+++ b"))
	before := owner.expect(t, owner.get("/tasks/"+child), http.StatusOK)

	owner.expect(t, owner.del("/columns/"+doomed), http.StatusNoContent)

	after := owner.expect(t, owner.get("/tasks/"+child), http.StatusOK)
	if after["column_id"] != kept || after["position"] != before["position"] {
		t.Fatalf("subtask moved with its parent's column: column %v→%v, position %v→%v",
			before["column_id"], after["column_id"], before["position"], after["position"])
	}
	if n, ok := countFiles(t, taskUploadDir(child)); !ok || n != 1 {
		t.Fatalf("subtask files touched: %d left (exists=%v)", n, ok)
	}
}

// TestPurgingADeletedBoardTakesEveryTaskAttachment: a deleted board keeps every
// file while it can still be restored, and the trash worker sweeps them all —
// subtasks included — when it finishes the delete. Another board's files stay.
func TestPurgingADeletedBoardTakesEveryTaskAttachment(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	col := s.col(t, 0)

	parent := mkTask(t, owner, s.Board, col, "Эпик")["id"].(string)
	attachFile(t, owner, parent, "план.md", []byte("# план"))
	child := owner.expect(t, owner.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Подзадача", "column_id": col, "parent_id": parent,
	}), http.StatusCreated)["id"].(string)
	attachFile(t, owner, child, "патч.diff", []byte("--- a\n+++ b"))

	other := mkBoard(t, owner, s.Project, "Доска рядом")
	otherCol := owner.get("/boards/" + other + "/columns").listBody(t)[0]["id"].(string)
	neighbour := mkTask(t, owner, other, otherCol, "Карточка на соседней доске")["id"].(string)
	attachFile(t, owner, neighbour, "сосед.txt", []byte("neighbour"))

	owner.expect(t, owner.del("/boards/"+s.Board), http.StatusNoContent)
	for _, id := range []string{parent, child} {
		if n, ok := countFiles(t, taskUploadDir(id)); !ok || n != 1 {
			t.Fatalf("task %s lost its files while the board can still be restored: %d (exists=%v)", id, n, ok)
		}
	}

	ageTrash(t, "boards", s.Board)
	testAPI.PurgeTrash(context.Background())

	for _, id := range []string{parent, child} {
		if n, ok := countFiles(t, taskUploadDir(id)); ok {
			t.Fatalf("task %s kept %d attachment file(s) after the purge", id, n)
		}
	}
	if n, ok := countFiles(t, taskUploadDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the purge reached a board that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingAProjectTakesEveryTaskAttachment: a project owns its boards, and
// its tasks are two cascades down — the sweep cannot read them off the id in the
// URL, it has to resolve them.
func TestDeletingAProjectTakesEveryTaskAttachment(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Карточка удаляемого проекта")["id"].(string)
	attachFile(t, owner, task, "устав.pdf", []byte("%PDF-1.7 ustav"))

	// A second project in the same workspace: its files must stay.
	otherProj := mkProject(t, owner, s.WS, s.Group, "Проект рядом")
	otherBoard := mkBoard(t, owner, otherProj, "Доска рядом")
	otherCol := owner.get("/boards/" + otherBoard + "/columns").listBody(t)[0]["id"].(string)
	neighbour := mkTask(t, owner, otherBoard, otherCol, "Карточка соседнего проекта")["id"].(string)
	attachFile(t, owner, neighbour, "сосед.txt", []byte("neighbour"))

	owner.expect(t, owner.del("/projects/"+s.Project), http.StatusNoContent)

	if n, ok := countFiles(t, taskUploadDir(task)); ok {
		t.Fatalf("project delete left %d file(s) no row points at", n)
	}
	if n, ok := countFiles(t, taskUploadDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the sweep reached a project that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingAWorkspaceTakesEveryTaskAttachment: the widest gesture of the four
// — three cascades from the id in the URL to a task — and the only one the owner
// alone can make, which is also the one after which nobody is left to notice.
func TestDeletingAWorkspaceTakesEveryTaskAttachment(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Карточка удаляемого пространства")["id"].(string)
	attachFile(t, owner, task, "архив.zip", []byte("PK\x03\x04"))

	// A second workspace of the same owner: its files must stay.
	other := mkStack(t, owner)
	neighbour := mkTask(t, owner, other.Board, other.col(t, 0), "Карточка соседнего пространства")["id"].(string)
	attachFile(t, owner, neighbour, "сосед.txt", []byte("neighbour"))

	owner.expect(t, owner.del("/workspaces/"+s.WS), http.StatusNoContent)

	if n, ok := countFiles(t, taskUploadDir(task)); ok {
		t.Fatalf("workspace delete left %d file(s) no row points at", n)
	}
	if n, ok := countFiles(t, taskUploadDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the sweep reached a workspace that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// ageTrash backdates a soft-deleted row past the trash retention, so the purge
// takes it on its next pass — without the test waiting a day or purging rows
// that parallel tests deleted a moment ago.
func ageTrash(t *testing.T, table, id string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		"UPDATE "+table+" SET deleted_at = now() - interval '2 days' WHERE id = $1 AND deleted_at IS NOT NULL", id); err != nil {
		t.Fatalf("age %s %s: %v", table, id, err)
	}
}

// TestDeletingAnEmptyContainerIsFine pins the common case: containers are deleted
// far more often empty than full, and a sweep that only works when there is
// something to sweep is one that fails the moment it matters.
func TestDeletingAnEmptyContainerIsFine(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)

	owner.expect(t, owner.del("/columns/"+s.col(t, 0)), http.StatusNoContent)
	owner.expect(t, owner.del("/boards/"+s.Board), http.StatusNoContent)
	owner.expect(t, owner.del("/projects/"+s.Project), http.StatusNoContent)
	owner.expect(t, owner.del("/workspaces/"+s.WS), http.StatusNoContent)
}
