// A GitLab link removed upstream has to disappear from BOTH tasks, and the run
// that notices it usually sees only one of the two issues (#2932).
//
// syncRelations writes the reverse edge itself when both ends live in the binding's
// own project, so a colleague's link shows on the other card even when that issue
// fell outside the run's scope. Pruning, though, is scoped to the iids the run
// actually inspected — and an incremental pull asks GitLab only for issues changed
// since the last sync. The forward row is then dropped while the mirror, filed
// under the *other* issue's iid, is not: ListTaskRelations reads relations from
// either direction, so the link the user deleted in GitLab keeps standing on both
// cards until some later run happens to touch the other end.
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestGitlabRelationDropDoesNotDependOnOtherEndBeingInDelta(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-relprune-user", "grp-relprune")

	f.addIssue(glIssue{IID: 1, Title: "Central"})
	f.addIssue(glIssue{IID: 2, Title: "Other end"})
	f.links = []glLink{{SrcIID: 1, DstIID: 2, LinkType: "relates_to", LinkID: 601}}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	integID := integ["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)

	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	central := taskByIid(t, tasks, 1)["id"].(string)
	other := taskByIid(t, tasks, 2)["id"].(string)

	// Precondition: the link is on both cards — #2 owes its row to the mirror,
	// since only #1's side declared it.
	if rels := c.get("/tasks/" + central + "/relations").listBody(t); len(rels) != 1 {
		t.Fatalf("relations on #1 after first sync = %d, want 1\n%v", len(rels), rels)
	}
	if rels := c.get("/tasks/" + other + "/relations").listBody(t); len(rels) != 1 {
		t.Fatalf("relations on #2 after first sync = %d, want 1 (mirror)\n%v", len(rels), rels)
	}

	// Upstream: the link is gone and only issue #1 changed. That is the ordinary
	// shape of an incremental pull — the delta carries the issue whose page the
	// user was on, not necessarily the one at the other end of the link.
	f.mu.Lock()
	f.links = nil
	f.mu.Unlock()
	is := f.findIssue(1)
	f.mu.Lock()
	is.Title = "Central v2"
	is.UpdatedAt = time.Now().UTC()
	f.mu.Unlock()

	triggerSync(t, c, s.WS, integID)
	runs := waitSyncRuns(t, c, s.WS, 2)
	if runs[0]["mode"] != "incremental" {
		t.Fatalf("second sync mode = %v, want incremental", runs[0]["mode"])
	}
	// The delta really did carry #1 alone: #2 is untouched, so it is dropped as
	// unchanged and its iid never reaches the relation pass.
	tasks = c.get("/boards/" + s.Board + "/tasks").listBody(t)
	if got := taskByIid(t, tasks, 1)["title"]; got != "Central v2" {
		t.Fatalf("issue #1 not in the delta (title = %v)", got)
	}
	if got := taskByIid(t, tasks, 2)["title"]; got != "Other end" {
		t.Fatalf("issue #2 unexpectedly re-synced (title = %v)", got)
	}

	if rels := c.get("/tasks/" + central + "/relations").listBody(t); len(rels) != 0 {
		t.Fatalf("relation still on #1 after the GitLab link was deleted: %v", rels)
	}
	if rels := c.get("/tasks/" + other + "/relations").listBody(t); len(rels) != 0 {
		t.Fatalf("mirror relation still on #2 after the GitLab link was deleted: %v", rels)
	}
}

// A link the run did NOT inspect must survive: the pruning below widens to the
// mirror of a stale row, and widening it too far would make every incremental
// pull erase the relations of every issue outside its delta.
func TestGitlabRelationsOutsideDeltaSurvive(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-relkeep-user", "grp-relkeep")

	f.addIssue(glIssue{IID: 1, Title: "Untouched A"})
	f.addIssue(glIssue{IID: 2, Title: "Untouched B"})
	f.addIssue(glIssue{IID: 3, Title: "The one that changes"})
	f.links = []glLink{{SrcIID: 1, DstIID: 2, LinkType: "relates_to", LinkID: 701}}

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	integID := integ["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)

	// A finished run is not proof the import landed: under load the first run
	// to finish was seen with the board still empty (#3123). Wait for the tasks.
	tasks := waitBoardTaskCount(t, c, s.Board, 3)
	a := taskByIid(t, tasks, 1)["id"].(string)
	b := taskByIid(t, tasks, 2)["id"].(string)

	// Only #3 changes; the link between #1 and #2 is still on record in GitLab.
	is := f.findIssue(3)
	f.mu.Lock()
	is.Title = "Changed"
	is.UpdatedAt = time.Now().UTC()
	f.mu.Unlock()

	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 2)

	if rels := c.get("/tasks/" + a + "/relations").listBody(t); len(rels) != 1 {
		t.Fatalf("relation on #1 lost by a pull that never looked at it: %v", rels)
	}
	if rels := c.get("/tasks/" + b + "/relations").listBody(t); len(rels) != 1 {
		t.Fatalf("mirror relation on #2 lost by a pull that never looked at it: %v", rels)
	}
	if r := c.get("/tasks/" + a + "/relations"); r.Status != http.StatusOK {
		t.Fatalf("relations read: status %d", r.Status)
	}
}

// waitBoardTaskCount polls a board until it lists at least n tasks.
func waitBoardTaskCount(t *testing.T, c *client, boardID string, n int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		tasks := c.get("/boards/" + boardID + "/tasks").listBody(t)
		if len(tasks) >= n || time.Now().After(deadline) {
			return tasks
		}
		time.Sleep(100 * time.Millisecond)
	}
}
