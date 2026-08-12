package main

import (
	"net/http"
	"testing"
)

// Batch B wraps several multi-statement mutations in a transaction. These tests
// exercise the wrapped handlers end-to-end: the risk of the change is that a
// rewrite breaks the happy path, so we assert each handler still produces a
// consistent result across both of its tables. (Rollback itself is delegated to
// pgx via API.inTx; the fixtures mkWorkspace/mkBoard already run the
// CreateWorkspace/CreateBoard wraps on every test that builds a stack.)

// CreateWorkspace must commit the workspace and its owner membership together —
// a workspace with no membership is invisible even to its creator, so the only
// way it shows up in the owner's list is if both rows landed.
func TestCreateWorkspaceMembershipCommitted(t *testing.T) {
	c := signup(t)
	wsID := mkWorkspace(t, c, "commit-check")

	list := c.get("/workspaces").listBody(t)
	found := false
	for _, w := range list {
		if w["id"] == wsID {
			found = true
		}
	}
	if !found {
		t.Fatalf("workspace %s not visible to its owner — membership not committed", wsID)
	}
}

// SetTagPrefixes is DELETE-then-UPSERT inside one transaction. A replace must be
// atomic: after replacing [type,app] with [prio] the project must show exactly
// the new set, never a leftover or a wiped-but-not-refilled state.
func TestSetTagPrefixesReplaceIsAtomic(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)

	put := func(prefixes []map[string]any) []map[string]any {
		r := c.put("/projects/"+s.Project+"/tag-prefixes", map[string]any{"prefixes": prefixes})
		if r.Status != http.StatusOK {
			t.Fatalf("set prefixes: status %d\n%s", r.Status, r.Body)
		}
		return c.get("/projects/" + s.Project + "/tag-prefixes").listBody(t)
	}

	got := put([]map[string]any{{"prefix": "type", "label": "Тип"}, {"prefix": "app", "label": "Прил"}})
	if len(got) != 2 {
		t.Fatalf("want 2 prefixes after first set, got %d: %v", len(got), got)
	}

	got = put([]map[string]any{{"prefix": "prio", "label": "Приоритет"}})
	if len(got) != 1 || got[0]["prefix"] != "prio" {
		t.Fatalf("replace not atomic: want exactly [prio], got %v", got)
	}
}

// TransferTask moves the task and its subtasks to the new board in one
// transaction — a subtask must never be stranded on the old board.
func TestTransferTaskMovesSubtasksTogether(t *testing.T) {
	c := signup(t)
	s := mkStack(t, c)
	col0 := s.col(t, 0)

	parent := mkTask(t, c, s.Board, col0, "parent")
	parentID := parent["id"].(string)
	sub := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
		"title": "child", "column_id": col0, "parent_id": parentID,
	}), http.StatusCreated)
	subID := sub["id"].(string)

	board2 := mkBoard(t, c, s.Project, "Доска-2")

	r := c.patch("/tasks/"+parentID+"/transfer", map[string]any{"board_id": board2})
	if r.Status != http.StatusOK {
		t.Fatalf("transfer: status %d\n%s", r.Status, r.Body)
	}

	for _, id := range []string{parentID, subID} {
		got := c.get("/tasks/" + id).mapBody(t)
		if got["board_id"] != board2 {
			t.Fatalf("task %s on board %v, want %s (subtask stranded?)", id, got["board_id"], board2)
		}
	}
}
