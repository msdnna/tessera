// The task and document trash, the project archive (migration 0086, #3001:
// #3004, #3005, #3006, #3120).
//
// A delete is immediate and reversible: the row gets deleted_at, vanishes for
// every reader, comes back whole with POST …/restore, and goes for good with
// DELETE …/purge or after 30 days in trash_purge. A project archive hides the
// project from every list but keeps it readable — and read-only.
package main

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// deleteForGood is the old one-gesture delete: into the trash, then out of it
// for good. The purge tests that pin what a delete leaves on disk use it — since
// 0086 the sweep happens at the purge.
func deleteForGood(t *testing.T, c *client, kind, id, query string) {
	t.Helper()
	c.expect(t, c.del("/"+kind+"/"+id+query), http.StatusNoContent)
	c.expect(t, c.del("/"+kind+"/"+id+"/purge"), http.StatusNoContent)
}

// ageTrashedTask moves a task's delete stamp — and the stamp of everything that
// went in with it — back by d, the way the trash worker tests age their rows.
func ageTrashedTask(t *testing.T, id string, d time.Duration) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE tasks SET deleted_at = deleted_at - make_interval(secs => $2)
		 WHERE deleted_at = (SELECT deleted_at FROM tasks WHERE id = $1)`, id, d.Seconds()); err != nil {
		t.Fatalf("age trashed task: %v", err)
	}
}

func ageTrashedDocument(t *testing.T, id string, d time.Duration) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(),
		`UPDATE documents SET deleted_at = deleted_at - make_interval(secs => $2)
		 WHERE deleted_at = (SELECT deleted_at FROM documents WHERE id = $1)`, id, d.Seconds()); err != nil {
		t.Fatalf("age trashed document: %v", err)
	}
}

func ids(rows []map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, r := range rows {
		if id, ok := r["id"].(string); ok {
			out[id] = true
		}
	}
	return out
}

func searchHits(t *testing.T, c *client, ws, q, key string) map[string]bool {
	t.Helper()
	res := c.expect(t, c.get("/workspaces/"+ws+"/search?q="+url.QueryEscape(q)), http.StatusOK)
	out := map[string]bool{}
	rows, _ := res[key].([]any)
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			out[m["id"].(string)] = true
		}
	}
	return out
}

func TestTrashedTaskVanishesEverywhereAndComesBackWithItsSubtree(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	col := s.col(t, 0)
	due := time.Now().Add(time.Hour).Format(time.RFC3339)
	root := owner.expect(t, owner.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Корзинный корень", "column_id": col, "due_date": due,
	}), http.StatusCreated)
	rootID := root["id"].(string)
	number := int(root["number"].(float64))
	child := mkSubtask(t, owner, s.Board, col, rootID, "Корзинный ребёнок")["id"].(string)
	grand := mkSubtask(t, owner, s.Board, col, child, "Корзинный внук")["id"].(string)
	// A subtask thrown away on its own before the root goes: an entry of its own.
	early := mkSubtask(t, owner, s.Board, col, rootID, "Корзинный ранний")["id"].(string)
	owner.expect(t, owner.post("/tasks/"+rootID+"/assignees", map[string]any{"user_id": owner.UserID}), http.StatusNoContent)
	owner.expect(t, owner.del("/tasks/"+early), http.StatusNoContent)
	time.Sleep(5 * time.Millisecond) // two distinct stamps

	owner.expect(t, owner.del("/tasks/"+rootID), http.StatusNoContent)

	for _, id := range []string{rootID, child, grand, early} {
		if r := owner.get("/tasks/" + id); r.Status != http.StatusNotFound {
			t.Fatalf("GET a trashed task %s: %d, want 404", id, r.Status)
		}
	}
	if r := owner.get("/workspaces/" + s.WS + "/tasks/by-number/" + itoa(number)); r.Status != http.StatusNotFound {
		t.Fatalf("by-number of a trashed task: %d, want 404", r.Status)
	}
	if ids(owner.get("/boards/" + s.Board + "/tasks").listBody(t))[rootID] {
		t.Fatal("trashed task still on the board")
	}
	if subs := ids(owner.get("/boards/" + s.Board + "/subtasks").listBody(t)); subs[child] || subs[grand] {
		t.Fatal("trashed subtasks still on the board")
	}
	if ids(owner.get("/workspaces/" + s.WS + "/tasks").listBody(t))[rootID] {
		t.Fatal("trashed task still in «Мои задачи»")
	}
	if searchHits(t, owner, s.WS, "Корзинный", "tasks")[rootID] {
		t.Fatal("trashed task still found by search")
	}
	// The scanners: a due task in the trash warns nobody.
	due2, err := testQueries.ListDueTasksForScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range due2 {
		if d.ID.String() == rootID {
			t.Fatal("due scanner still sees a trashed task")
		}
	}
	// Writes on a trashed task are writes on nothing.
	if r := owner.patch("/tasks/"+rootID+"/move", map[string]any{"column_id": col}); r.Status != http.StatusNotFound {
		t.Fatalf("move a trashed task: %d, want 404", r.Status)
	}

	// The board's trash lists the root and the early subtask, not the subtree
	// that went in with the root.
	trash := owner.get("/boards/" + s.Board + "/trash").listBody(t)
	got := ids(trash)
	if !got[rootID] || !got[early] || got[child] || got[grand] || len(trash) != 2 {
		t.Fatalf("trash lists %v, want root and the early subtask", got)
	}
	for _, r := range trash {
		if r["id"] == rootID {
			if r["deleted_by_name"] == "" || r["subtask_count"].(float64) != 1 {
				t.Fatalf("trash row %v: want who deleted it and 1 subtask with it", r)
			}
		}
	}

	restored := owner.expect(t, owner.post("/tasks/"+rootID+"/restore", nil), http.StatusOK)
	if restored["deleted_at"] != nil || restored["id"] != rootID {
		t.Fatalf("restore answered %v", restored)
	}
	for _, id := range []string{rootID, child, grand} {
		owner.expect(t, owner.get("/tasks/"+id), http.StatusOK)
	}
	// What was in the trash before the root stays there.
	if r := owner.get("/tasks/" + early); r.Status != http.StatusNotFound {
		t.Fatalf("early subtask came back with the root: %d", r.Status)
	}
	// …and comes back under its (live again) parent on its own restore.
	back := owner.expect(t, owner.post("/tasks/"+early+"/restore", nil), http.StatusOK)
	if back["parent_id"] != rootID {
		t.Fatalf("early subtask restored with parent %v, want %s", back["parent_id"], rootID)
	}
	// Restoring a live task is not a thing.
	if r := owner.post("/tasks/"+rootID+"/restore", nil); r.Status != http.StatusNotFound {
		t.Fatalf("restore of a live task: %d, want 404", r.Status)
	}
	// The archive's PATCH …/restore is a different gesture and still works.
	owner.expect(t, owner.patch("/tasks/"+rootID+"/archive", nil), http.StatusNoContent)
	owner.expect(t, owner.patch("/tasks/"+rootID+"/restore", nil), http.StatusNoContent)
}

// A subtask restored while its parent is still in the trash comes back as a
// card: under a parent nobody can see it would be drawn nowhere.
func TestRestoredSubtaskOfATrashedParentBecomesACard(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	col := s.col(t, 0)
	parent := mkTask(t, owner, s.Board, col, "Родитель")["id"].(string)
	child := mkSubtask(t, owner, s.Board, col, parent, "Ребёнок")["id"].(string)
	owner.expect(t, owner.del("/tasks/"+child), http.StatusNoContent)
	owner.expect(t, owner.del("/tasks/"+parent), http.StatusNoContent)

	back := owner.expect(t, owner.post("/tasks/"+child+"/restore", nil), http.StatusOK)
	if back["parent_id"] != nil {
		t.Fatalf("restored under a trashed parent: parent_id %v", back["parent_id"])
	}
	if !ids(owner.get("/boards/" + s.Board + "/tasks").listBody(t))[child] {
		t.Fatal("restored subtask is not a card on the board")
	}
}

func TestPurgedTaskIsGoneForGood(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	task := mkTask(t, owner, s.Board, s.col(t, 0), "На выброс")["id"].(string)
	attachFile(t, owner, task, "a.txt", []byte("hello"))

	// A live task cannot be purged — only what is already in the trash.
	if r := owner.del("/tasks/" + task + "/purge"); r.Status != http.StatusNotFound {
		t.Fatalf("purge of a live task: %d, want 404", r.Status)
	}
	owner.expect(t, owner.del("/tasks/"+task), http.StatusNoContent)
	// In the trash the attachments are kept: they come back with the task.
	if n, ok := countFiles(t, taskUploadDir(task)); !ok || n == 0 {
		t.Fatalf("soft delete swept the attachments: %d (exists=%v)", n, ok)
	}
	owner.expect(t, owner.del("/tasks/"+task+"/purge"), http.StatusNoContent)
	if n, _ := countFiles(t, taskUploadDir(task)); n != 0 {
		t.Fatalf("purge left %d attachment file(s)", n)
	}
	if r := owner.post("/tasks/"+task+"/restore", nil); r.Status != http.StatusNotFound {
		t.Fatalf("restore after purge: %d, want 404", r.Status)
	}
	if ids(owner.get("/boards/" + s.Board + "/trash").listBody(t))[task] {
		t.Fatal("purged task still in the trash")
	}
}

// trash_purge keeps a task 30 days, then removes it with its subtree and files.
func TestTrashWorkerPurgesTasksAfterThirtyDays(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	col := s.col(t, 0)
	old := mkTask(t, owner, s.Board, col, "Старая")["id"].(string)
	oldChild := mkSubtask(t, owner, s.Board, col, old, "Старая подзадача")["id"].(string)
	attachFile(t, owner, oldChild, "c.txt", []byte("x"))
	fresh := mkTask(t, owner, s.Board, col, "Свежая")["id"].(string)
	owner.expect(t, owner.del("/tasks/"+old), http.StatusNoContent)
	owner.expect(t, owner.del("/tasks/"+fresh), http.StatusNoContent)
	ageTrashedTask(t, old, 31*24*time.Hour)
	ageTrashedTask(t, fresh, 29*24*time.Hour)

	testAPI.PurgeTrash(context.Background())

	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM tasks WHERE id = ANY($1::uuid[])`, []string{old, oldChild}).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d task row(s) outlived the 30 days", n)
	}
	if c, _ := countFiles(t, taskUploadDir(oldChild)); c != 0 {
		t.Fatalf("purged subtask kept %d file(s)", c)
	}
	// 29 days: still in the trash, still restorable.
	owner.expect(t, owner.post("/tasks/"+fresh+"/restore", nil), http.StatusOK)
}

func TestTaskTrashNeedsMembership(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	stranger := signup(t)
	s := mkStack(t, owner)
	task := mkTask(t, owner, s.Board, s.col(t, 0), "Чужая")["id"].(string)
	owner.expect(t, owner.del("/tasks/"+task), http.StatusNoContent)
	for _, r := range []resp{
		stranger.get("/boards/" + s.Board + "/trash"),
		stranger.post("/tasks/"+task+"/restore", nil),
		stranger.del("/tasks/" + task + "/purge"),
	} {
		if r.Status != http.StatusForbidden && r.Status != http.StatusNotFound {
			t.Fatalf("stranger reached the trash: %d", r.Status)
		}
	}
	owner.expect(t, owner.post("/tasks/"+task+"/restore", nil), http.StatusOK)
}

// A reminder pinned to a trashed task does not fire.
func TestReminderOfATrashedTaskDoesNotFire(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	task := mkTask(t, owner, s.Board, s.col(t, 0), "С напоминанием")["id"].(string)
	rem := owner.expect(t, owner.post("/reminders", map[string]any{
		"remind_at": time.Now().Add(-time.Minute).Format(time.RFC3339),
		"message":   "не забыть", "task_id": task,
	}), http.StatusCreated)["id"].(string)
	owner.expect(t, owner.del("/tasks/"+task), http.StatusNoContent)
	due, err := testQueries.ListDueReminders(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range due {
		if r.ID.String() == rem {
			t.Fatal("reminder of a trashed task is due for delivery")
		}
	}
}

// ── project archive (#3005) ────────────────────────────────────────────────

func TestArchivedProjectIsHiddenReadOnlyAndRestorable(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	col := s.col(t, 0)
	task := mkTask(t, owner, s.Board, col, "Архивная задача")
	taskID := task["id"].(string)
	owner.expect(t, owner.post("/tasks/"+taskID+"/assignees", map[string]any{"user_id": owner.UserID}), http.StatusNoContent)
	member := addMember(t, owner, s.WS)

	// Owner/admin only.
	if r := member.post("/projects/"+s.Project+"/archive", nil); r.Status != http.StatusForbidden {
		t.Fatalf("member archived a project: %d, want 403", r.Status)
	}
	arch := owner.expect(t, owner.post("/projects/"+s.Project+"/archive", nil), http.StatusOK)
	if arch["archived_at"] == nil {
		t.Fatalf("archive answered %v", arch)
	}
	// Gone from the tree, the search, «Мои задачи»; listed in the archive.
	if ids(owner.get("/workspaces/" + s.WS + "/projects").listBody(t))[s.Project] {
		t.Fatal("archived project still in the tree")
	}
	if !ids(owner.get("/workspaces/" + s.WS + "/projects?archived=1").listBody(t))[s.Project] {
		t.Fatal("archived project missing from the archive list")
	}
	if searchHits(t, owner, s.WS, "Архивная", "tasks")[taskID] {
		t.Fatal("task of an archived project found by search")
	}
	if searchHits(t, owner, s.WS, "Доска", "boards")[s.Board] {
		t.Fatal("board of an archived project found by search")
	}
	if ids(owner.get("/workspaces/" + s.WS + "/tasks").listBody(t))[taskID] {
		t.Fatal("task of an archived project in «Мои задачи»")
	}
	// Still readable…
	owner.expect(t, owner.get("/projects/"+s.Project), http.StatusOK)
	owner.expect(t, owner.get("/boards/"+s.Board), http.StatusOK)
	owner.expect(t, owner.get("/tasks/"+taskID), http.StatusOK)
	if !ids(owner.get("/boards/" + s.Board + "/tasks").listBody(t))[taskID] {
		t.Fatal("archived project's board lost its tasks")
	}
	// …but not writable.
	for name, r := range map[string]resp{
		"update":   owner.patch("/tasks/"+taskID, map[string]any{"title": "Правка"}),
		"move":     owner.patch("/tasks/"+taskID+"/move", map[string]any{"column_id": s.col(t, 1)}),
		"delete":   owner.del("/tasks/" + taskID),
		"create":   owner.post("/boards/"+s.Board+"/tasks", map[string]any{"title": "Новая", "column_id": col}),
		"column":   owner.post("/boards/"+s.Board+"/columns", map[string]any{"name": "Ещё"}),
		"board":    owner.post("/projects/"+s.Project+"/boards", map[string]any{"name": "Ещё доска"}),
		"comment":  owner.post("/tasks/"+taskID+"/comments", map[string]any{"body": "привет"}),
		"rename-c": owner.patch("/columns/"+col, map[string]any{"name": "X"}),
	} {
		if r.Status != http.StatusConflict {
			t.Fatalf("%s in an archived project: %d, want 409\n%s", name, r.Status, r.Body)
		}
	}
	// A second archive finds nothing to do.
	if r := owner.post("/projects/"+s.Project+"/archive", nil); r.Status != http.StatusNotFound {
		t.Fatalf("second archive: %d, want 404", r.Status)
	}

	back := owner.expect(t, owner.post("/projects/"+s.Project+"/restore", nil), http.StatusOK)
	if back["archived_at"] != nil {
		t.Fatalf("restore answered %v", back)
	}
	if !ids(owner.get("/workspaces/" + s.WS + "/projects").listBody(t))[s.Project] {
		t.Fatal("restored project not back in the tree")
	}
	owner.expect(t, owner.patch("/tasks/"+taskID+"/move", map[string]any{"column_id": s.col(t, 1)}), http.StatusOK)
	if !ids(owner.get("/workspaces/" + s.WS + "/tasks").listBody(t))[taskID] {
		t.Fatal("restored project's task not back in «Мои задачи»")
	}
}

func TestArchivedProjectIsOutOfTheDueScanner(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	task := owner.expect(t, owner.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Срочно", "column_id": s.col(t, 0), "due_date": time.Now().Add(time.Hour).Format(time.RFC3339),
	}), http.StatusCreated)["id"].(string)
	owner.expect(t, owner.post("/projects/"+s.Project+"/archive", nil), http.StatusOK)
	due, err := testQueries.ListDueTasksForScan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range due {
		if d.ID.String() == task {
			t.Fatal("due scanner still sees a task of an archived project")
		}
	}
}

// ── documents trash (#3006) ────────────────────────────────────────────────

func TestTrashedDocumentKeepsItsLinksAndComesBack(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s := mkStack(t, owner)
	parent := owner.expect(t, owner.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "Корзинный регламент"}), http.StatusCreated)["id"].(string)
	child := owner.expect(t, owner.post("/workspaces/"+s.WS+"/documents",
		map[string]any{"title": "Приложение", "parent_id": parent}), http.StatusCreated)["id"].(string)
	task := mkTask(t, owner, s.Board, s.col(t, 0), "По регламенту")["id"].(string)
	owner.expect(t, owner.post("/documents/"+parent+"/tasks", map[string]any{"task_id": task}), http.StatusCreated)

	// A container still asks for ?recursive=true.
	if r := owner.del("/documents/" + parent); r.Status != http.StatusConflict {
		t.Fatalf("delete of a container without recursive: %d, want 409", r.Status)
	}
	owner.expect(t, owner.del("/documents/"+parent+"?recursive=true"), http.StatusNoContent)

	for _, id := range []string{parent, child} {
		if r := owner.get("/documents/" + id); r.Status != http.StatusNotFound {
			t.Fatalf("GET a trashed document: %d, want 404", r.Status)
		}
	}
	if got := ids(owner.get("/workspaces/" + s.WS + "/documents").listBody(t)); got[parent] || got[child] {
		t.Fatal("trashed documents still in the tree")
	}
	if searchHits(t, owner, s.WS, "Корзинный", "documents")[parent] {
		t.Fatal("trashed document found by search")
	}
	if len(owner.get("/tasks/"+task+"/documents").listBody(t)) != 0 {
		t.Fatal("task still lists a trashed document")
	}
	trash := ids(owner.get("/workspaces/" + s.WS + "/documents/trash").listBody(t))
	if !trash[parent] || trash[child] {
		t.Fatalf("documents trash lists %v, want the root only", trash)
	}

	owner.expect(t, owner.post("/documents/"+parent+"/restore", nil), http.StatusOK)
	owner.expect(t, owner.get("/documents/"+child), http.StatusOK)
	if len(owner.get("/tasks/"+task+"/documents").listBody(t)) != 1 {
		t.Fatal("the link to the task did not come back with the document")
	}

	// Purge: only from the trash, and then for good.
	if r := owner.del("/documents/" + child + "/purge"); r.Status != http.StatusNotFound {
		t.Fatalf("purge of a live document: %d, want 404", r.Status)
	}
	owner.expect(t, owner.del("/documents/"+child), http.StatusNoContent)
	owner.expect(t, owner.del("/documents/"+child+"/purge"), http.StatusNoContent)
	if r := owner.post("/documents/"+child+"/restore", nil); r.Status != http.StatusNotFound {
		t.Fatalf("restore after purge: %d, want 404", r.Status)
	}
}

func TestTrashWorkerPurgesDocumentsAfterThirtyDays(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	ws := mkWorkspace(t, owner, "WS "+t.Name())
	parent := owner.expect(t, owner.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Старый"}), http.StatusCreated)["id"].(string)
	child := owner.expect(t, owner.post("/workspaces/"+ws+"/documents",
		map[string]any{"title": "Вложенный", "parent_id": parent}), http.StatusCreated)["id"].(string)
	owner.expect(t, owner.del("/documents/"+parent+"?recursive=true"), http.StatusNoContent)
	ageTrashedDocument(t, parent, 31*24*time.Hour)

	testAPI.PurgeTrash(context.Background())

	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM documents WHERE id = ANY($1::uuid[])`, []string{parent, child}).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d document row(s) outlived the 30 days", n)
	}
}

// A mirror thrown into the trash stays there: the sync neither updates it out
// from under the trash nor imports its issue anew (the link survives), and a
// restore hands it back to the sync.
func TestGitlabSyncLeavesATrashedMirrorInTheTrash(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-trash-user", "grp-trash")
	f.addIssue(glIssue{IID: 1, Title: "Зеркало"})
	f.addIssue(glIssue{IID: 2, Title: "Сосед"})
	connectGitlab(t, c, f)
	integID := createIntegration(t, c, s.WS, s.Board, f, nil)["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)
	mirror := taskByIid(t, c.get("/boards/"+s.Board+"/tasks").listBody(t), 1)["id"].(string)

	c.expect(t, c.del("/tasks/"+mirror), http.StatusNoContent)
	is := f.findIssue(1)
	f.mu.Lock()
	is.Title = "Зеркало (правка в GitLab)"
	is.UpdatedAt = time.Now().Add(time.Minute)
	f.mu.Unlock()
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 2)

	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	if len(tasks) != 1 {
		t.Fatalf("board has %d tasks after the sync, want 1 (the mirror stays in the trash)", len(tasks))
	}
	got, err := testQueries.GetTaskAny(context.Background(), mustUUID(t, mirror))
	if err != nil {
		t.Fatal(err)
	}
	if got.DeletedAt == nil || got.Title != "Зеркало" {
		t.Fatalf("trashed mirror touched by the sync: deleted_at=%v title=%q", got.DeletedAt, got.Title)
	}

	c.expect(t, c.post("/tasks/"+mirror+"/restore", nil), http.StatusOK)
	f.mu.Lock()
	is.UpdatedAt = time.Now().Add(2 * time.Minute)
	f.mu.Unlock()
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 3)
	if back := c.expect(t, c.get("/tasks/"+mirror), http.StatusOK); back["title"] != "Зеркало (правка в GitLab)" {
		t.Fatalf("restored mirror not synced again: %v", back["title"])
	}
}
