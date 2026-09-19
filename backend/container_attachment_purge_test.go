package main

import (
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
// Each test keeps a sibling container alive. A sweep that cleared the upload root
// — the cheapest wrong fix — would satisfy every "the files are gone" assertion
// and fail exactly these.

// TestDeletingAColumnTakesItsTasksAttachments: board_columns.id is the FK tasks
// cascade from, so dropping a column silently deletes every card standing in it.
func TestDeletingAColumnTakesItsTasksAttachments(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	doomed, kept := s.col(t, 0), s.col(t, 1)

	task := mkTask(t, owner, s.Board, doomed, "Карточка в удаляемой колонке")["id"].(string)
	attachFile(t, owner, task, "смета.pdf", []byte("%PDF-1.7 smeta"))
	neighbour := mkTask(t, owner, s.Board, kept, "Карточка в соседней колонке")["id"].(string)
	attachFile(t, owner, neighbour, "сосед.txt", []byte("neighbour"))

	owner.expect(t, owner.del("/columns/"+doomed), http.StatusNoContent)

	if got := owner.get("/tasks/" + task); got.Status != http.StatusNotFound {
		t.Fatalf("task in a deleted column still readable: %d", got.Status)
	}
	if n, ok := countFiles(t, taskUploadDir(task)); ok {
		t.Fatalf("column delete left %d file(s) no row points at", n)
	}
	if n, ok := countFiles(t, taskUploadDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the sweep reached a column that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingAColumnTakesSubtasksStandingElsewhere: the sweep reads the doomed
// tasks off column_id, on the grounds that a subtree is wholly inside whichever
// container holds its root. That holds for a board, a project and a workspace —
// MoveSubtreeToBoard exists to keep it holding — but not for a column: a subtask
// is a card of its own, the board draws it and a drag moves it, so it stands
// wherever it was dragged and its parent stands somewhere else.
//
// tasks.parent_id is ON DELETE CASCADE, so deleting the parent's column takes
// that subtask too, out of a column nobody deleted and at any depth. Its
// task_attachments rows go with it — and its directory is not in the sweep's
// list, so the bytes stay on disk with nothing left pointing at them.
func TestDeletingAColumnTakesSubtasksStandingElsewhere(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	doomed, kept, third := s.col(t, 0), s.col(t, 1), s.col(t, 2)

	parent := mkTask(t, owner, s.Board, doomed, "Эпик в удаляемой колонке")["id"].(string)
	attachFile(t, owner, parent, "план.md", []byte("# план"))

	subtask := func(t *testing.T, parentID, title, column string) string {
		t.Helper()
		id := owner.expect(t, owner.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": doomed, "parent_id": parentID,
		}), http.StatusCreated)["id"].(string)
		// Dragged out of the parent's column, which is the whole point.
		owner.expect(t, owner.patch("/tasks/"+id+"/move",
			map[string]any{"column_id": column}), http.StatusOK)
		return id
	}
	// A child one column over and a grandchild two: the cascade is not depth-limited,
	// so neither is the sweep that has to match it.
	child := subtask(t, parent, "Подзадача в работе", kept)
	attachFile(t, owner, child, "патч.diff", []byte("--- a\n+++ b"))
	grandchild := subtask(t, child, "Под-подзадача на ревью", third)
	attachFile(t, owner, grandchild, "лог.txt", []byte("trace"))

	neighbour := mkTask(t, owner, s.Board, kept, "Карточка в соседней колонке")["id"].(string)
	attachFile(t, owner, neighbour, "сосед.txt", []byte("neighbour"))

	owner.expect(t, owner.del("/columns/"+doomed), http.StatusNoContent)

	// The cascade really does reach them — otherwise the file assertions below
	// would pass for the wrong reason.
	for _, id := range []string{child, grandchild} {
		if got := owner.get("/tasks/" + id); got.Status != http.StatusNotFound {
			t.Fatalf("subtask %s survived its parent's column delete: %d", id, got.Status)
		}
	}
	for _, id := range []string{parent, child, grandchild} {
		if n, ok := countFiles(t, taskUploadDir(id)); ok {
			t.Fatalf("task %s kept %d attachment file(s) no row points at", id, n)
		}
	}
	if n, ok := countFiles(t, taskUploadDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the sweep reached a card that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingABoardTakesEveryTaskAttachment: a board is the unit a team retires,
// and it holds the whole tree — subtasks included, each with its own directory.
func TestDeletingABoardTakesEveryTaskAttachment(t *testing.T) {
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

	// A second board in the same project: its files must stay.
	other := mkBoard(t, owner, s.Project, "Доска рядом")
	otherCol := owner.get("/boards/" + other + "/columns").listBody(t)[0]["id"].(string)
	neighbour := mkTask(t, owner, other, otherCol, "Карточка на соседней доске")["id"].(string)
	attachFile(t, owner, neighbour, "сосед.txt", []byte("neighbour"))

	owner.expect(t, owner.del("/boards/"+s.Board), http.StatusNoContent)

	for _, id := range []string{parent, child} {
		if n, ok := countFiles(t, taskUploadDir(id)); ok {
			t.Fatalf("task %s kept %d attachment file(s) after the board delete", id, n)
		}
	}
	if n, ok := countFiles(t, taskUploadDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the sweep reached a board that was not deleted: %d file(s) left (exists=%v)", n, ok)
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
