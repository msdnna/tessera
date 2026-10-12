package main

import (
	"net/http"
	"path/filepath"
	"testing"
)

// What a deleted task leaves on disk (#2932).
//
// UploadAttachment picks its layout for exactly one reason, and says so:
// "Store under <uploadDir>/<taskID>/<uuid><ext> so paths never collide and
// removing a task's files is a single directory." The removing half was never
// written. DeleteTask runs `DELETE FROM tasks WHERE id = $1` and answers 204;
// the task_attachments rows go with it by cascade, and with them the only record
// of where the bytes live — storage_path is written by UploadAttachment and read
// by nothing else afterwards. Files up to 25 MB each, no cap on how many.
//
// Subtasks make it plural rather than theoretical: a subtask is a task with its
// own id and therefore its own directory, and deleting the parent takes the
// whole subtree through the same one gesture.

// taskUploadDir is where a task's attachments live (handlers.UploadAttachment).
func taskUploadDir(taskID string) string {
	return filepath.Join(testUploadDir, taskID)
}

// attachFile uploads one attachment to a task.
func attachFile(t *testing.T, c *client, taskID, name string, body []byte) {
	t.Helper()
	c.expect(t, uploadFile(t, c, "/tasks/"+taskID+"/attachments", "file", name, body),
		http.StatusCreated)
}

// TestDeletingATaskTakesItsAttachmentsWithIt: the bytes behind a deleted task's
// files must not outlive the rows that named them.
//
// Deleting a task is the ordinary way to get rid of one, and far more common
// than deleting a document or a conference. After the 204 nothing in the
// database points at <uploadDir>/<taskID>/ — no row, no sweep, no screen. The
// author cannot learn the directory is still full, and no code path ever looks
// at it again.
func TestDeletingATaskTakesItsAttachmentsWithIt(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	col := s.col(t, 0)

	task := mkTask(t, owner, s.Board, col, "Задача с файлами")["id"].(string)
	attachFile(t, owner, task, "смета.pdf", []byte("%PDF-1.7 smeta"))
	attachFile(t, owner, task, "логи.txt", []byte("panic: boom"))

	dir := taskUploadDir(task)
	if n, ok := countFiles(t, dir); !ok || n != 2 {
		t.Fatalf("attachments did not land: %d file(s) in %s (exists=%v)", n, dir, ok)
	}

	// A second task on the same board, so "swept" cannot quietly mean "swept the
	// upload root". The directories are siblings under one root, and the cheapest
	// wrong fix — clearing that root — would pass every other assertion here.
	neighbour := mkTask(t, owner, s.Board, col, "Соседняя задача")["id"].(string)
	attachFile(t, owner, neighbour, "сосед.txt", []byte("neighbour"))

	deleteForGood(t, owner, "tasks", task, "")

	// The task is gone from the API...
	if got := owner.get("/tasks/" + task); got.Status != http.StatusNotFound {
		t.Fatalf("deleted task still readable: %d", got.Status)
	}
	// ...so nothing can name these files any more.
	if n, ok := countFiles(t, dir); ok {
		t.Fatalf("upload directory survived the task with %d file(s) no row points at", n)
	}
	// The task next door still has its attachment.
	if n, ok := countFiles(t, taskUploadDir(neighbour)); !ok || n != 1 {
		t.Fatalf("the sweep reached a task that was not deleted: %d file(s) left (exists=%v)", n, ok)
	}
}

// TestDeletingATaskTakesEverySubtasksAttachments: deleting a parent is one
// gesture that removes a whole tree, and a sweep wired only to the id in the URL
// would leave every subtask's files behind — the rows are gone by cascade, so
// those directories are exactly as unreachable as the parent's.
func TestDeletingATaskTakesEverySubtasksAttachments(t *testing.T) {
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
	grandchild := owner.expect(t, owner.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Под-подзадача", "column_id": col, "parent_id": child,
	}), http.StatusCreated)["id"].(string)
	attachFile(t, owner, grandchild, "скрин.png", []byte("\x89PNG\r\n\x1a\nx"))

	for _, id := range []string{parent, child, grandchild} {
		if n, ok := countFiles(t, taskUploadDir(id)); !ok || n != 1 {
			t.Fatalf("attachment of %s did not land: %d (exists=%v)", id, n, ok)
		}
	}

	deleteForGood(t, owner, "tasks", parent, "")

	// Nesting is not depth-limited, so the grandchild is the assertion that a
	// one-level sweep would fail.
	for _, id := range []string{parent, child, grandchild} {
		if n, ok := countFiles(t, taskUploadDir(id)); ok {
			t.Fatalf("task %s kept %d attachment file(s) after the subtree delete", id, n)
		}
	}
}

// TestDeletingATaskKeepsDetachedSubtasksAttachments is the other half, and the
// reason the sweep cannot walk the subtree the delete would have taken.
//
// ?subtasks=detach re-parents the children to null first: they survive the
// request and stay on the board, so their files are still reachable and must
// stay. A sweep that read the tree before the detach — or that simply removed
// every descendant it found at request time — would delete the attachments of
// tasks the user deliberately kept, and unlike the leak that failure is loud and
// unrecoverable.
func TestDeletingATaskKeepsDetachedSubtasksAttachments(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	col := s.col(t, 0)

	parent := mkTask(t, owner, s.Board, col, "Эпик на расформирование")["id"].(string)
	attachFile(t, owner, parent, "устав.pdf", []byte("%PDF-1.7 ustav"))
	child := owner.expect(t, owner.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Выживающая подзадача", "column_id": col, "parent_id": parent,
	}), http.StatusCreated)["id"].(string)
	attachFile(t, owner, child, "важное.txt", []byte("keep me"))

	deleteForGood(t, owner, "tasks", parent, "?subtasks=detach")

	// The child outlived the request...
	if got := owner.get("/tasks/" + child); got.Status != http.StatusOK {
		t.Fatalf("detached subtask did not survive the delete: %d", got.Status)
	}
	// ...so its attachment must have too.
	if n, ok := countFiles(t, taskUploadDir(child)); !ok || n != 1 {
		t.Fatalf("attachment of a detached subtask was swept: %d file(s) left (exists=%v)", n, ok)
	}
	// The parent's own files still go.
	if n, ok := countFiles(t, taskUploadDir(parent)); ok {
		t.Fatalf("deleted parent kept %d attachment file(s)", n)
	}
}

// TestDeletingATaskWithoutAttachmentsIsFine pins the overwhelmingly common
// delete — a task nobody attached anything to. A sweep that only works when
// there is something to sweep is one that fails the moment it matters.
func TestDeletingATaskWithoutAttachmentsIsFine(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)

	task := mkTask(t, owner, s.Board, s.col(t, 0), "Задача без файлов")["id"].(string)
	deleteForGood(t, owner, "tasks", task, "")

	if n, ok := countFiles(t, taskUploadDir(task)); ok {
		t.Fatalf("directory appeared for a task with no attachments: %d entr(ies)", n)
	}
}
