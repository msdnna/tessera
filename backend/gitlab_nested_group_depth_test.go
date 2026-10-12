package main

import (
	"net/http"
	"testing"

	"tessera/internal/gitlab"
)

// TestGitlabNestedGroupsMirrorGrandchild pins that a GitLab hierarchy deeper than
// one level is mirrored whole.
//
// The pull splits the issues of a run in two: mirrorIssues walks the ones nobody
// claimed, and a grouped parent's children are mirrored by mirrorChildren under
// the task the parent became. `claimed` is built by fetchGroupedChildren, which
// asks EVERY grouped issue of the run for its children — including an issue that
// is itself somebody's child. So in ЭПИК → СОСТАВНАЯ → ВНУК the grandchild is
// claimed by the middle issue and skipped by mirrorIssues ("synced as a subtask
// under its parent"), while mirrorChildren — which mirrors the middle issue —
// never descends into ITS children.
//
// Nobody mirrors the grandchild, then. On a first sync it gets no task at all:
// no card, no subtask, no journal line — the issue simply is not in Tessera, and
// the delete-detection pass still sees it in the fetch, so nothing ever reports
// it missing. On a board that was already mirrored flat it is worse: the task
// exists and silently stops being updated, since every later run skips it the
// same way.
//
// Nesting on the Tessera side has no depth limit (applyParent hangs a subtask off
// a subtask, MoveSubtreeToBoard and ArchiveTaskCascade walk the whole tree), so
// the shape is representable — it was only the pull that stopped at one level.
func TestGitlabNestedGroupsMirrorGrandchild(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-nested-user", "grp-nested")

	// Two levels of grouping: !10 groups !11, and !11 groups !12.
	f.addIssue(glIssue{IID: 10, Title: "Эпик", Labels: []glLabel{{Title: gitlab.DefaultGroupLabel}}})
	f.addIssue(glIssue{IID: 11, Title: "Составная задача", Labels: []glLabel{{Title: gitlab.DefaultGroupLabel}}})
	f.addIssue(glIssue{IID: 12, Title: "Внук"})
	// The control lives on its own: a plain issue with no hierarchy at all, so a
	// board that came back empty is told apart from a board that lost one branch.
	f.addIssue(glIssue{IID: 20, Title: "Одиночная задача"})
	f.children[10] = []int64{11}
	f.children[11] = []int64{12}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	// subtaskTitles lists the titles of one task's children, by id.
	subtaskTitles := func(t *testing.T, taskID string) []string {
		t.Helper()
		detail := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
		kids, _ := detail["subtasks"].([]any)
		out := make([]string, 0, len(kids))
		for _, k := range kids {
			kid, _ := k.(map[string]any)
			title, _ := kid["title"].(string)
			out = append(out, title)
		}
		return out
	}
	// subtaskByTitle finds one child of a task, or fails.
	subtaskByTitle := func(t *testing.T, taskID, title string) map[string]any {
		t.Helper()
		detail := c.expect(t, c.get("/tasks/"+taskID), http.StatusOK)
		kids, _ := detail["subtasks"].([]any)
		for _, k := range kids {
			kid, _ := k.(map[string]any)
			if kid["title"] == title {
				return kid
			}
		}
		t.Fatalf("task %s has no subtask «%s»; it has %v", taskID, title, subtaskTitles(t, taskID))
		return nil
	}

	cards := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	epic := taskByIid(t, cards, 10)["id"].(string)

	t.Run("the control issue is mirrored as a card", func(t *testing.T) {
		// Keeps the asserts below honest: the fixture does sync, and this board's
		// cards are read the way the others are.
		if got := taskByIid(t, cards, 20)["title"]; got != "Одиночная задача" {
			t.Fatalf("control card title = %v", got)
		}
	})

	t.Run("the first level is mirrored under the epic", func(t *testing.T) {
		// The known-good depth. If this breaks, the grandchild asserts say nothing.
		if got := subtaskTitles(t, epic); len(got) != 1 || got[0] != "Составная задача" {
			t.Fatalf("epic subtasks = %v, want [Составная задача]", got)
		}
	})

	t.Run("the grandchild is mirrored under the middle issue", func(t *testing.T) {
		middle := subtaskByTitle(t, epic, "Составная задача")["id"].(string)
		if got := subtaskTitles(t, middle); len(got) != 1 || got[0] != "Внук" {
			t.Fatalf("middle issue subtasks = %v, want [Внук]", got)
		}
	})

	t.Run("the grandchild is not left standing on the board", func(t *testing.T) {
		// The other way the depth could be "handled": dumping the grandchild at
		// top level. That loses the hierarchy just as thoroughly, so it is pinned
		// as a failure of its own rather than folded into the assert above.
		for _, card := range c.get("/boards/" + s.Board + "/tasks").listBody(t) {
			if card["title"] == "Внук" {
				t.Fatalf("grandchild issue is drawn as a top-level card: %v", card)
			}
		}
	})
}
