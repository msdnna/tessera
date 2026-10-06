package main

import (
	"net/http"
	"testing"

	"tessera/internal/gitlab"
)

// TestGitlabArchivedParentKeepsOpenChildVisible pins that an *active* subtask
// whose parent has been archived is still drawn somewhere.
//
// The manual archive keeps the invariant by construction: ArchiveTaskCascade
// takes the whole subtree, and ?subtasks=detach re-parents the children to the
// root instead. The GitLab pull has no such pairing — applyClosedPolicy runs
// per issue, and closed_policy=archive_closed_sprints is the *default* for every
// integration — so a parent issue closed inside a closed milestone is archived
// on its own while a child issue that is still open stays active.
//
// That leaves a row no screen fetches. The board asks for top-level cards
// (`t.parent_id IS NULL`) and hangs subtasks off the parent card in the same
// payload (subtasksByParent), and the parent is archived; the archive view
// filters on archived_at IS NOT NULL, and the child's is NULL. The task is on
// nobody's board and in nobody's archive — the same disappearance
// TestArchiveCarriesWholeSubtree pinned for grandchildren.
func TestGitlabArchivedParentKeepsOpenChildVisible(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-orphan-user", "grp-orphan")

	// A finished sprint: the parent issue is closed and its milestone is closed
	// with it. The child rolled over — it is open and carries no milestone.
	sprint := &glMilestone{ID: 500, IID: 5, Title: "Sprint 1", State: "closed"}
	f.addIssue(glIssue{
		IID: 10, Title: "Закрытый родитель", State: "closed", Milestone: sprint,
		Labels: []glLabel{{Title: gitlab.DefaultGroupLabel}},
	})
	f.addIssue(glIssue{IID: 11, Title: "Открытый ребёнок", State: "opened"})
	f.children[10] = []int64{11}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, map[string]any{
		"closed_policy": "archive_closed_sprints",
	})

	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	// The archive view carries no gl_iid (it selects the task row alone), so the
	// parent is found by title — the id then comes off it.
	var parent map[string]any
	for _, r := range c.get("/boards/" + s.Board + "/archive").listBody(t) {
		if r["title"] == "Закрытый родитель" {
			parent = r
		}
	}
	if parent == nil {
		t.Fatalf("closed parent in a closed sprint is not in the archive")
	}
	if parent["archived_at"] == nil {
		t.Fatalf("archive served an active row: %v", parent)
	}

	detail := c.expect(t, c.get("/tasks/"+parent["id"].(string)), http.StatusOK)
	kids, _ := detail["subtasks"].([]any)
	if len(kids) != 1 {
		t.Fatalf("parent has %d subtasks, want 1\n%v", len(kids), detail["subtasks"])
	}
	child, _ := kids[0].(map[string]any)
	childID, _ := child["id"].(string)
	if child["archived_at"] != nil {
		t.Fatalf("open child issue was archived along with its parent: %v", child)
	}

	t.Run("the active board draws the orphan as a card", func(t *testing.T) {
		tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
		for _, r := range tasks {
			if r["id"] == childID {
				return
			}
		}
		t.Fatalf("active child %s is not among the board's cards: %v", childID, tasks)
	})

	t.Run("it is not also served as somebody's subtask", func(t *testing.T) {
		// Otherwise it renders twice on a client that merges the two payloads —
		// and the per-column header counts (cards + all their subtasks) count it
		// twice as well.
		for _, r := range c.get("/boards/" + s.Board + "/subtasks").listBody(t) {
			if r["id"] == childID {
				t.Fatalf("orphaned subtask %s is served by both list endpoints", childID)
			}
		}
	})

	t.Run("the archive still lists the parent alone", func(t *testing.T) {
		seen := map[string]bool{}
		for _, r := range c.get("/boards/" + s.Board + "/archive").listBody(t) {
			seen[r["id"].(string)] = true
		}
		if !seen[parent["id"].(string)] {
			t.Fatalf("archive lost the parent: %v", seen)
		}
		if seen[childID] {
			t.Fatalf("archive lists the active child %s: %v", childID, seen)
		}
	})

	t.Run("restoring the parent takes the child back under it", func(t *testing.T) {
		c.expect(t, c.patch("/tasks/"+parent["id"].(string)+"/restore", map[string]any{}), http.StatusNoContent)
		for _, r := range c.get("/boards/" + s.Board + "/tasks").listBody(t) {
			if r["id"] == childID {
				t.Fatalf("child %s still stands as a top-level card under a restored parent", childID)
			}
		}
		found := false
		for _, r := range c.get("/boards/" + s.Board + "/subtasks").listBody(t) {
			if r["id"] == childID {
				found = true
			}
		}
		if !found {
			t.Fatalf("child %s is no longer served as a subtask of the restored parent", childID)
		}
	})
}
