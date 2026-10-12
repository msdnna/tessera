package main

import (
	"net/http"
	"testing"
	"time"

	"tessera/internal/gitlab"
)

// TestGitlabSyncRefusesParentRing pins that a pull cannot weld two tasks into a
// parent ring.
//
// Every other door onto parent_id refuses to attach a task below itself:
// applyParent walks TaskAncestorIDs first («cannot attach a task to itself or to
// one of its own subtasks»), and that guard exists because a ring is invisible —
// ListBoardTasksWithMeta draws only parent_id IS NULL, so cards reachable from no
// root are drawn on no board, and nothing in the UI can undo what nothing draws.
// The sync's own re-parent (syncOneIssue) called SetTaskParent straight, with no
// such check.
//
// Normally the pull cannot produce one anyway: a task nobody claimed is detached
// to top level BEFORE its children are placed, so an issue that becomes the parent
// of its own Tessera parent loses its stale parent in the same run. The exception
// is the branch that was added to survive a GitLab hiccup (#2592): when a grouped
// parent's children cannot be listed, its current subtasks are «distrusted» and
// left exactly where they are. Then one half of the run keeps А under Б while the
// other half puts Б under А — and both cards drop off the board for good, since
// every later run sees the claim already satisfied and writes nothing.
func TestGitlabSyncRefusesParentRing(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-ring-user", "grp-ring")

	// Two grouped issues and two plain ones. Б groups «Пункт Б» from the start, so
	// the run that loses its children list has something to distrust.
	f.addIssue(glIssue{IID: 50, Title: "Составная А", Labels: []glLabel{{Title: gitlab.DefaultGroupLabel}}})
	f.addIssue(glIssue{IID: 51, Title: "Составная Б", Labels: []glLabel{{Title: gitlab.DefaultGroupLabel}}})
	f.addIssue(glIssue{IID: 52, Title: "Пункт Б"})
	f.addIssue(glIssue{IID: 53, Title: "Обычная задача"})
	f.children[51] = []int64{52}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	integID := integ["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)

	// A finished run is not yet a visible import under load (#3123).
	cards := waitBoardTaskCount(t, c, s.Board, 2)
	taskA := taskByIid(t, cards, 50)["id"].(string)
	taskB := taskByIid(t, cards, 51)["id"].(string)
	plain := taskByIid(t, cards, 53)["id"].(string)

	// The user files А under Б by hand. Allowed — Б is nowhere below А right now.
	c.expect(t, c.patch("/tasks/"+taskA+"/parent", map[string]any{"parent_id": taskB}), http.StatusOK)

	// Meanwhile in GitLab А becomes the grouped parent of Б (and of the plain
	// issue), and Б's own Hierarchy widget starts answering with an error.
	f.mu.Lock()
	f.children[50] = []int64{51, 53}
	f.childrenErr[51] = true
	f.mu.Unlock()
	// Well clear of the first sync: the fake reports updatedAt with second
	// precision, so a bump inside the same second reads as unchanged and the whole
	// second run is dropped as an empty delta (#2736's «green means nothing» in
	// miniature — the asserts below would then pass on a sync that never ran).
	for _, iid := range []int64{50, 51} {
		is := f.findIssue(iid)
		f.mu.Lock()
		is.UpdatedAt = time.Now().Add(2 * time.Minute).UTC()
		f.mu.Unlock()
	}

	triggerSync(t, c, s.WS, integID)
	runs := waitSyncRuns(t, c, s.WS, 2)
	acts := c.get("/workspaces/" + s.WS + "/gitlab/sync-runs/" + runs[0]["id"].(string) + "/actions").itemsBody(t)

	// parentOf reads one task's parent id ("" for a top-level card).
	parentOf := func(t *testing.T, id string) string {
		t.Helper()
		detail := c.expect(t, c.get("/tasks/"+id), http.StatusOK)
		p, _ := detail["parent_id"].(string)
		return p
	}
	// topLevel reports whether the board draws this id as a card of its own.
	topLevel := func(t *testing.T, id string) bool {
		t.Helper()
		for _, card := range c.get("/boards/" + s.Board + "/tasks").listBody(t) {
			if card["id"] == id {
				return true
			}
		}
		return false
	}

	t.Run("контроль: второй прогон действительно отработал", func(t *testing.T) {
		// Without this the whole test could pass on a sync that did nothing: an
		// incremental pull with an empty delta touches no task, and «А осталась под
		// Б» is exactly what a no-op leaves behind. The distrust line is the run's
		// own record of having asked Б for its children and lost the answer.
		for _, a := range acts {
			if a["op"] == "skip" && a["gl_iid"] == float64(51) {
				return
			}
		}
		t.Fatalf("прогон не дошёл до иерархии #51: actions = %v", acts)
	})

	t.Run("контроль: законный захват в том же прогоне применён", func(t *testing.T) {
		// The plain issue became a child of А in GitLab and is below nothing —
		// that re-parent must still land, or the guard below is just «never
		// re-parent anything».
		if got := parentOf(t, plain); got != taskA {
			t.Fatalf("обычная задача: parent_id = %q, want %s (под А)", got, taskA)
		}
	})

	t.Run("контроль: подзадача Б не тронута потерянным списком детей", func(t *testing.T) {
		// The #2592 behaviour itself: Б's children query failed, so «Пункт Б»
		// stays where it is instead of being read as detached.
		detail := c.expect(t, c.get("/tasks/"+taskB), http.StatusOK)
		kids, _ := detail["subtasks"].([]any)
		found := false
		for _, k := range kids {
			kid, _ := k.(map[string]any)
			if kid["title"] == "Пункт Б" {
				found = true
			}
		}
		if !found {
			t.Fatalf("«Пункт Б» больше не под Б: subtasks = %v", kids)
		}
	})

	t.Run("кольца нет: подъём по родителям упирается в корень", func(t *testing.T) {
		seen := map[string]bool{}
		for id := taskA; id != ""; id = parentOf(t, id) {
			if seen[id] {
				t.Fatalf("кольцо родителей: %s встречен дважды при подъёме от А", id)
			}
			seen[id] = true
		}
	})

	t.Run("доска по-прежнему рисует обе задачи", func(t *testing.T) {
		// The user-visible half of the same fact: in a ring neither А nor Б is a
		// top-level card and neither is drawn under a card, so both vanish.
		if !topLevel(t, taskB) {
			t.Fatalf("Б пропала с доски: parent_id = %q", parentOf(t, taskB))
		}
		if p := parentOf(t, taskA); p != taskB {
			t.Fatalf("А больше не под Б: parent_id = %q", p)
		}
	})
}
