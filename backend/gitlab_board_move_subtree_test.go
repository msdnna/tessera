package main

import (
	"net/http"
	"testing"
	"time"
)

// TestGitlabBoardRuleCarriesSubtreeAlong pins that when the pull moves a mirrored
// task to another board, its subtasks come along.
//
// A "board" label rule routes an issue to a board of its own: put «B: вторая» on
// an already-mirrored issue and the next sync writes the new board_id onto the
// task. But a Tessera-side subtask hung under that task — a checklist item someone
// typed in the modal, with no GitLab issue of its own — is never touched by the
// sync, so it keeps the board its parent has just left.
//
// That row is then drawn nowhere. ListBoardTasksWithMeta on the old board skips
// it (its parent is alive, so it is not a top-level card) and ListBoardSubtasksWithMeta
// hands it over without the parent card it renders under; on the new board it is
// not selected at all. And board_id/column_id are ON DELETE CASCADE, so deleting
// that stale column later takes the subtask with it.
//
// Every other door onto board_id already carries the subtree — applyParent
// (MoveSubtreeToBoard on a cross-board attach) and TransferTask (which detaches
// instead). The pull was the one that did not.
func TestGitlabBoardRuleCarriesSubtreeAlong(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	other := mkBoard(t, c, s.Project, "Вторая доска")
	f := newFakeGitlab(t, "gl-boardmove-user", "grp-boardmove")

	// !10 is the issue that will be routed away; !20 is the control that stays put.
	f.addIssue(glIssue{IID: 10, Title: "Переезжает на другую доску", State: "opened"})
	f.addIssue(glIssue{IID: 20, Title: "Остаётся на своей доске", State: "opened"})

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, map[string]any{
		"label_rules": map[string]any{
			"rules": []any{map[string]any{
				"match": "B: ", "match_type": "prefix", "action": "board",
				"value_map": map[string]string{"вторая": other},
			}},
		},
	})
	integID := integ["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)

	// Both issues land on the integration's own board, and each gets a Tessera-only
	// subtask — the kind the sync never sees, because it has no GitLab link.
	cards := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	mover := taskByIid(t, cards, 10)["id"].(string)
	stayer := taskByIid(t, cards, 20)["id"].(string)
	kidOf := map[string]string{}
	for _, parent := range []string{mover, stayer} {
		kid := c.expect(t, c.post("/boards/"+s.Board+"/tasks", map[string]any{
			"title": "Пункт чек-листа", "column_id": s.col(t, 0), "parent_id": parent,
		}), http.StatusCreated)
		kidOf[parent] = kid["id"].(string)
	}

	// The remote change under test: !10 gains the board label.
	is := f.findIssue(10)
	f.mu.Lock()
	is.Labels = []glLabel{{Title: "B: вторая"}}
	is.UpdatedAt = time.Now().UTC()
	f.mu.Unlock()
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 2)

	boardOf := func(taskID string) string {
		t.Helper()
		return c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)["board_id"].(string)
	}

	t.Run("the board rule moved the parent", func(t *testing.T) {
		// The control that keeps the assert below honest: it proves the rule fires
		// at all, so a subtask left behind is about the subtree and not the fixture.
		if got := boardOf(mover); got != other {
			t.Fatalf("parent board = %s, want the rule's board %s", got, other)
		}
	})

	t.Run("its subtask moved with it", func(t *testing.T) {
		if got := boardOf(kidOf[mover]); got != other {
			t.Fatalf("subtask stayed on board %s while its parent left for %s", got, other)
		}
	})

	t.Run("no orphan is drawn on the board it left", func(t *testing.T) {
		// The half that hurts: a subtask on the old board is handed to that board's
		// subtask list with no parent card to render under, so it is drawn nowhere.
		subs := c.get("/boards/" + s.Board + "/subtasks").listBody(t)
		for _, sub := range subs {
			if sub["id"] == kidOf[mover] {
				t.Fatalf("subtask %s is still served by the old board's subtask list", kidOf[mover])
			}
		}
		orphan := c.get("/boards/" + other + "/subtasks").listBody(t)
		found := false
		for _, sub := range orphan {
			if sub["id"] == kidOf[mover] {
				found = true
			}
		}
		if !found {
			t.Fatalf("subtask %s is on neither board's subtask list", kidOf[mover])
		}
	})

	t.Run("a task no rule routed keeps its board and its subtask", func(t *testing.T) {
		// The contrast: the sync rewrites board_id for every issue it touches, so
		// carrying the subtree must not turn an ordinary update into a move.
		if got := boardOf(stayer); got != s.Board {
			t.Fatalf("untouched parent board = %s, want %s", got, s.Board)
		}
		if got := boardOf(kidOf[stayer]); got != s.Board {
			t.Fatalf("untouched subtask board = %s, want %s", got, s.Board)
		}
	})
}
