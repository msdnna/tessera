// A recurrence must hand the next occurrence the WHOLE checklist, not its first
// level.
//
// Subtask nesting has no depth limit — CreateTask hangs a subtask off a subtask
// without complaint, and the board draws every level (the column counters walk
// the whole tree) — so a repeating task decomposed two levels deep is an ordinary
// task, not an exotic one. The create_new branch rebuilt the checklist from
// ListSubtasks(src), which returns the DIRECT children only, and hung each copy
// under the clone without ever asking those children for children of their own.
// The reschedule branch edits the task in place and carries the whole subtree
// untouched, so the two branches disagreed: the first duplicate collapsed a
// two-level plan into a one-level one, and the steps below the first level were
// not archived, not moved and not reported — they simply were not there, in this
// occurrence and in every one after it.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestRecurrenceClonesNestedSubtasks(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)

	due := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	rfc := func(ts time.Time) string { return ts.Format(time.RFC3339) }
	mkSub := func(parentID, title string) string {
		t.Helper()
		sub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": title, "column_id": s.col(t, 0), "parent_id": parentID,
		}), http.StatusCreated)
		return sub["id"].(string)
	}

	// ── the create_new branch: a two-level checklist under a repeating task ──
	orig := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Закрытие месяца", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	origID := orig["id"].(string)

	branchID := mkSub(origID, "Свести цифры")
	leafID := mkSub(branchID, "Выгрузить выписки")
	mkSub(branchID, "Сверить с кассой")
	// A flat item alongside the nested branch: the clone has to keep copying the
	// first level exactly as it did, so a regression here is not mistaken for the
	// fix working.
	mkSub(origID, "Разослать отчёт")

	// The deep item carries a tag and an assignee — the first level copies both,
	// and a level that arrives bare is its own kind of loss.
	tag := c.expect(t, c.post("/projects/"+s.Project+"/tags",
		map[string]any{"name": "закрытие", "color": "#3b82f6"}), http.StatusCreated)
	c.expect(t, c.post("/tasks/"+leafID+"/tags",
		map[string]any{"tag_id": tag["id"]}), http.StatusNoContent)

	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Закрытие месяца", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "monthly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)

	clone := findClone(t, c, s.Board, origID, "Закрытие месяца")
	if !hasSubtaskTitled(clone, "Свести цифры") || !hasSubtaskTitled(clone, "Разослать отчёт") {
		t.Fatalf("the duplicate lost a first-level checklist item: %v", clone["subtasks"])
	}
	cloneBranchID := subtaskByTitle(t, clone, "Свести цифры")["id"].(string)
	if cloneBranchID == branchID {
		t.Fatalf("the duplicate took the original's subtask instead of copying it")
	}
	cloneBranch := c.expect(t, c.get("/tasks/"+cloneBranchID), http.StatusOK)
	if !hasSubtaskTitled(cloneBranch, "Выгрузить выписки") ||
		!hasSubtaskTitled(cloneBranch, "Сверить с кассой") {
		t.Fatalf("the duplicate collapsed the nested level: %v", cloneBranch["subtasks"])
	}
	// Copied, not stolen: the deep items are new rows, and the first occurrence
	// still holds its own.
	cloneLeaf := subtaskByTitle(t, cloneBranch, "Выгрузить выписки")
	if cloneLeaf["id"] == leafID {
		t.Fatalf("the duplicate moved the nested item out of the first occurrence")
	}
	firstBranch := c.expect(t, c.get("/tasks/"+branchID), http.StatusOK)
	if !hasSubtaskTitled(firstBranch, "Выгрузить выписки") {
		t.Fatalf("the first occurrence lost its nested checklist: %v", firstBranch["subtasks"])
	}
	// The nested copy is a full one, the way the first level is.
	if tags, _ := cloneLeaf["tag_ids"].([]any); len(tags) != 1 {
		t.Fatalf("the nested copy arrived without its tag: %v", cloneLeaf)
	}

	// ── control: an archived item stays out at depth too ──
	// The first level already skips archived items; the recursion must not hand
	// the deeper levels a weaker rule.
	deepArchived := mkSub(branchID, "Отменённая выгрузка")
	c.expect(t, c.patch("/tasks/"+deepArchived+"/archive", nil), http.StatusNoContent)
	c.expect(t, c.patch("/tasks/"+origID, map[string]any{
		"title": "Закрытие месяца", "due_date": rfc(due.AddDate(0, 1, 0)),
		"recurrence": map[string]any{"freq": "monthly", "interval": 1, "create_new": true},
		"completed":  true,
	}), http.StatusOK)
	if got := countArchivedTitled(t, c, s.Board, "Отменённая выгрузка"); got != 1 {
		t.Fatalf("archive holds %d copies of the deep archived item, want 1", got)
	}

	// ── control: the reschedule branch keeps the whole tree ──
	// It edits the task in place and never rebuilds the checklist, so this is what
	// the duplicate has to match.
	daily := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "Ежедневный статус", "column_id": s.col(t, 0), "due_date": rfc(due),
	}), http.StatusCreated)
	dailyID := daily["id"].(string)
	dailyBranch := mkSub(dailyID, "Собрать блокеры")
	mkSub(dailyBranch, "Опросить команду")

	c.expect(t, c.patch("/tasks/"+dailyID, map[string]any{
		"title": "Ежедневный статус", "due_date": rfc(due),
		"recurrence": map[string]any{"freq": "daily", "interval": 1},
		"completed":  true,
	}), http.StatusOK)

	rescheduled := c.expect(t, c.get("/tasks/"+dailyBranch), http.StatusOK)
	if !hasSubtaskTitled(rescheduled, "Опросить команду") {
		t.Fatalf("a reschedule lost the nested item: %v", rescheduled["subtasks"])
	}
}
