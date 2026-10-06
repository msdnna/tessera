package main

import (
	"fmt"
	"net/http"
	"testing"

	"tessera/internal/gitlab"
)

// TestArchiveDetachPushesChildDetachToGitlab pins that «убрать в архив, оставив
// подзадачи на доске» reaches GitLab as well.
//
// Archiving a parent with ?subtasks=detach is the user saying "keep these on the
// board" — DetachChildren re-parents them to the root. The very same gesture made
// through PATCH /tasks/:id/parent (parent_id: null) enqueues a child_detach, so
// GitLab's hierarchy follows; the archive door did not, and neither did
// DELETE /tasks/:id?subtasks=detach.
//
// That is not a cosmetic divergence, because the pull reads the hierarchy back:
// GitLab still holds the child under the parent's work item, so the next sync hits
// the re-parent branch (`parent.known && !sameUUIDPtr(...)` in gitlab.go) and puts
// the child back under a parent that is now ARCHIVED. An active child of an
// archived parent is on nobody's board and in nobody's archive — the exact
// disappearance TestGitlabArchivedParentKeepsOpenChildVisible pins. The user's
// choice is silently undone and the card vanishes.
//
// Same class the code already refuses to half-do one door over: ClearGitlabTaskGroup
// declines to ungroup a parent that still has GitLab-linked subtasks ("detach them
// first") for precisely this reason.
func TestArchiveDetachPushesChildDetachToGitlab(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-archdetach-user", "grp-archdetach")

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
		// push_children is what the child_* outbox kinds are gated on; without it
		// checkChildGate refuses every direction and the test would prove nothing.
		"writeback": map[string]any{"enabled": true, "push_children": true},
	})
	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	// The grouped parents are the board's cards; each child arrives as their subtask.
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

	t.Run("the /parent door queues a detach", func(t *testing.T) {
		// The control that keeps a red assert below honest: it proves the stand
		// (integration, links, grouped parent, push_children) can produce the row
		// at all, so a missing row elsewhere is about that door and not the fixture.
		pair := ids[30]
		c.expect(t, c.patch("/tasks/"+pair.child+"/parent", map[string]any{"parent_id": nil}), http.StatusOK)
		if !hasChildDetach(pair.child) {
			t.Fatalf("detaching via /parent queued no %s: %v", gitlab.KindChildDetach, writebackRows(t, pair.child))
		}
	})

	t.Run("archiving with ?subtasks=detach queues one too", func(t *testing.T) {
		pair := ids[10]
		c.expect(t, c.patch("/tasks/"+pair.parent+"/archive?subtasks=detach", map[string]any{}), http.StatusNoContent)

		// Tessera's half of the gesture, first — the child is back on the board.
		detail := c.expect(t, c.get("/tasks/"+pair.child), http.StatusOK)
		if detail["parent_id"] != nil {
			t.Fatalf("child was not detached from the archived parent: %v", detail["parent_id"])
		}
		if !hasChildDetach(pair.child) {
			t.Fatalf("archive+detach queued no %s for child %s: %v",
				gitlab.KindChildDetach, pair.child, writebackRows(t, pair.child))
		}
	})

	t.Run("archiving the whole subtree queues nothing", func(t *testing.T) {
		// The contrast: without ?subtasks=detach the child goes into the archive
		// WITH its parent and comes back with it, so GitLab's hierarchy is still
		// the truth — dropping the issue to top-level there would be wrong.
		pair := ids[20]
		c.expect(t, c.patch("/tasks/"+pair.parent+"/archive", map[string]any{}), http.StatusNoContent)
		if hasChildDetach(pair.child) {
			t.Fatalf("cascade archive queued a %s for child %s: %v",
				gitlab.KindChildDetach, pair.child, writebackRows(t, pair.child))
		}
	})
}

// TestDeleteDetachPushesChildDetachToGitlab is the same contract for the delete
// door. DELETE /tasks/:id?subtasks=detach takes the parent away for good and keeps
// its children on the board; in GitLab they stay nested under the (still open)
// parent issue, and the next pull re-parents them under a task that no longer
// exists — it opens a fresh one for the parent issue and adopts them into it.
func TestDeleteDetachPushesChildDetachToGitlab(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-deldetach-user", "grp-deldetach")

	f.addIssue(glIssue{
		IID: 40, Title: "Родитель под снос", State: "opened",
		Labels: []glLabel{{Title: gitlab.DefaultGroupLabel}},
	})
	f.addIssue(glIssue{IID: 41, Title: "Ребёнок остаётся", State: "opened"})
	f.children[40] = []int64{41}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, map[string]any{
		"writeback": map[string]any{"enabled": true, "push_children": true},
	})
	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	parentID := taskByIid(t, c.get("/boards/"+s.Board+"/tasks").listBody(t), 40)["id"].(string)
	detail := c.expect(t, c.get("/tasks/"+parentID), http.StatusOK)
	kids, _ := detail["subtasks"].([]any)
	if len(kids) != 1 {
		t.Fatalf("parent has %d subtasks, want 1\n%v", len(kids), detail["subtasks"])
	}
	childID := kids[0].(map[string]any)["id"].(string)

	c.expect(t, c.del("/tasks/"+parentID+"?subtasks=detach"), http.StatusNoContent)

	if d := c.expect(t, c.get("/tasks/"+childID), http.StatusOK); d["parent_id"] != nil {
		t.Fatalf("child was not detached from the deleted parent: %v", d["parent_id"])
	}
	if _, ok := writebackRows(t, childID)[gitlab.KindChildDetach]; !ok {
		t.Fatalf("delete+detach queued no %s for child %s: %v",
			gitlab.KindChildDetach, childID, writebackRows(t, childID))
	}
}
