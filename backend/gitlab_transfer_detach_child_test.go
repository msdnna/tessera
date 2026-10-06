package main

import (
	"fmt"
	"net/http"
	"testing"

	"tessera/internal/gitlab"
)

// TestTransferPushesChildDetachToGitlab pins that moving a subtask to another
// board reaches GitLab as a detach.
//
// PATCH /tasks/:id/transfer clears parent_id unconditionally (the TransferTask
// statement sets it to NULL) — a task cannot keep a parent it is no longer
// standing beside, and the handler's own neighbours say so: "TransferTask
// refuses to leave one behind by detaching". So a transfer IS a detach, the same
// gesture PATCH /tasks/:id/parent makes with parent_id: null, and that door
// enqueues a child_detach.
//
// The transfer door did not, and GitLab is not a passive mirror here: the work
// item stays under the parent's, so the next pull hits the re-parent branch
// (`parent.known && !sameUUIDPtr(...)` in gitlab.go) and calls SetTaskParent —
// which writes parent_id, board_id AND column_id together. The card is dragged
// back under its old parent on the old board, and the transfer the user asked
// for is silently undone by a sync they did not run.
//
// Same class as the archive/delete door (TestArchiveDetachPushesChildDetachToGitlab).
func TestTransferPushesChildDetachToGitlab(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-transferdetach-user", "grp-transferdetach")

	// Three grouped parents with one child each: the case under test and two
	// controls, all pulled in by a single sync.
	pairs := []struct{ parent, child int64 }{{10, 11}, {20, 21}, {30, 31}}
	for _, p := range pairs {
		f.addIssue(glIssue{
			IID: p.parent, Title: fmt.Sprintf("Родитель !%d", p.parent), State: "opened",
			Labels: []glLabel{{Title: gitlab.DefaultGroupLabel}},
		})
		f.addIssue(glIssue{IID: p.child, Title: fmt.Sprintf("Ребёнок !%d", p.child), State: "opened"})
		f.children[p.parent] = []int64{p.child}
	}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, map[string]any{
		// push_children gates every child_* outbox kind; without it checkChildGate
		// refuses both directions and the test would prove nothing.
		"writeback": map[string]any{"enabled": true, "push_children": true},
	})
	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	cards := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	ids := map[int64]struct{ parent, child string }{}
	for _, p := range pairs {
		parentID := taskByIid(t, cards, float64(p.parent))["id"].(string)
		detail := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
		kids, _ := detail["subtasks"].([]any)
		if len(kids) != 1 {
			t.Fatalf("issue !%d: parent has %d subtasks, want 1\n%v", p.parent, len(kids), detail["subtasks"])
		}
		kid, _ := kids[0].(map[string]any)
		ids[p.parent] = struct{ parent, child string }{parentID, kid["id"].(string)}
	}

	// hasChildDetach reports whether the task's outbox holds a child_detach row.
	hasChildDetach := func(taskID string) bool {
		_, ok := writebackRows(t, taskID)[gitlab.KindChildDetach]
		return ok
	}

	board2 := mkBoard(t, c, s.Project, "Доска-приёмник")

	t.Run("the /parent door queues a detach", func(t *testing.T) {
		// The control that keeps the red assert below honest: it proves the stand
		// (integration, links, grouped parent, push_children) can produce the row at
		// all, so a missing row elsewhere is about that door and not the fixture.
		pair := ids[30]
		c.expect(t, c.patch("/tasks/"+pair.child+"/parent", map[string]any{"parent_id": nil}), http.StatusOK)
		if !hasChildDetach(pair.child) {
			t.Fatalf("detaching via /parent queued no %s: %v", gitlab.KindChildDetach, writebackRows(t, pair.child))
		}
	})

	t.Run("transferring a subtask to another board queues one too", func(t *testing.T) {
		pair := ids[10]
		got := c.expect(t, c.patch("/tasks/"+pair.child+"/transfer",
			map[string]any{"board_id": board2}), http.StatusOK)
		// The transfer really did detach it locally — without this the missing
		// outbox row below would be correct behaviour rather than a bug.
		if got["parent_id"] != nil {
			t.Fatalf("fixture: transfer left parent_id = %v, want nil", got["parent_id"])
		}
		if got["board_id"] != board2 {
			t.Fatalf("fixture: transfer left board_id = %v, want %s", got["board_id"], board2)
		}
		if !hasChildDetach(pair.child) {
			t.Fatalf("transfer queued no %s — GitLab keeps the child under !10, and the next "+
				"pull drags it back onto the old board: %v", gitlab.KindChildDetach, writebackRows(t, pair.child))
		}
	})

	t.Run("transferring a top-level card queues nothing", func(t *testing.T) {
		// A card with no parent detaches from nobody: the door must not invent a
		// detach for every transfer.
		pair := ids[20]
		c.expect(t, c.patch("/tasks/"+pair.parent+"/transfer",
			map[string]any{"board_id": board2}), http.StatusOK)
		if hasChildDetach(pair.parent) {
			t.Fatalf("transferring a parentless task queued a %s: %v",
				gitlab.KindChildDetach, writebackRows(t, pair.parent))
		}
	})
}
