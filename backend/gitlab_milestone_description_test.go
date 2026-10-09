// A GitLab milestone is mirrored as a native project milestone, and GitLab is
// the source of truth for its synced fields. The description used to be left
// out of the pull — neither queried nor written — so every sync reset the native
// milestone's description to "", wiping even the one Tessera had pushed (#2968).
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestGitlabSyncMirrorsMilestoneDescription(t *testing.T) {
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-msdesc-user", "grp-msdesc")
	f.addIssue(glIssue{
		IID: 1, Title: "Ship login",
		Milestone: &glMilestone{ID: 51, IID: 5, Title: "Sprint 5", Description: "Goals: ship login", State: "active"},
	})

	connectGitlab(t, c, f)
	integID := createIntegration(t, c, s.WS, s.Board, f, nil)["id"].(string)

	description := func() any {
		t.Helper()
		for _, m := range c.get("/projects/" + s.Project + "/milestones").listBody(t) {
			if m["title"] == "Sprint 5" {
				return m["description"]
			}
		}
		t.Fatalf("milestone 'Sprint 5' was not mirrored")
		return nil
	}
	// A milestone edit in GitLab doesn't touch the issue's updatedAt, so only a
	// full sweep sees it — that is what the "Полная синхронизация" action is for.
	fullSync := func(runs int) {
		t.Helper()
		if r := c.post("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/sync?mode=full", nil); r.Status != http.StatusAccepted {
			t.Fatalf("full sync: status %d\n%s", r.Status, r.Body)
		}
		waitSyncRuns(t, c, s.WS, runs)
	}
	setRemote := func(desc string) {
		f.mu.Lock()
		f.issues[0].Milestone.Description = desc
		f.mu.Unlock()
	}

	// First sync creates the native milestone with GitLab's description.
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)
	if got := description(); got != "Goals: ship login" {
		t.Fatalf("after create: description = %v, want %q", got, "Goals: ship login")
	}

	// An edit in GitLab reaches the existing native milestone.
	setRemote("Goals: ship login and signup")
	fullSync(2)
	if got := description(); got != "Goals: ship login and signup" {
		t.Fatalf("after update: description = %v, want %q", got, "Goals: ship login and signup")
	}

	// GitLab wins even when it clears the text, like it does for title and dates.
	setRemote("")
	fullSync(3)
	if got := description(); got != "" {
		t.Fatalf("after clear: description = %v, want empty", got)
	}
}

// A GitLab issue moved between milestones is journalled with the stage ids
// (#2992) — the stage page books it as scope change — while the first import
// of a task writes nothing: it says nothing about when the task joined.
func TestGitlabSyncJournalsMilestoneMove(t *testing.T) {
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-msmove-user", "grp-msmove")
	f.addIssue(glIssue{
		IID: 1, Title: "Ship login",
		Milestone: &glMilestone{ID: 61, IID: 6, Title: "Sprint 6", State: "active"},
	})
	connectGitlab(t, c, f)
	integID := createIntegration(t, c, s.WS, s.Board, f, nil)["id"].(string)
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)

	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	id := tasks[0]["id"].(string)
	if n := countEvents(t, c, id, "milestone"); n != 0 {
		t.Fatalf("first import journalled %d milestone entries, want none", n)
	}
	ids := map[string]string{}
	stageIDs := func() {
		for _, m := range c.get("/projects/" + s.Project + "/milestones").listBody(t) {
			ids[m["title"].(string)] = m["id"].(string)
		}
	}
	stageIDs()

	f.mu.Lock()
	f.issues[0].Milestone = &glMilestone{ID: 71, IID: 7, Title: "Sprint 7", State: "active"}
	f.issues[0].UpdatedAt = time.Now().UTC()
	f.mu.Unlock()
	if r := c.post("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/sync?mode=full", nil); r.Status != http.StatusAccepted {
		t.Fatalf("full sync: status %d\n%s", r.Status, r.Body)
	}
	waitSyncRuns(t, c, s.WS, 2)
	stageIDs()
	d := lastTagEvent(t, c, id, "milestone")
	if d["milestone_id"] != ids["Sprint 7"] || d["from_id"] != ids["Sprint 6"] {
		t.Fatalf("sync move journal = %v, want Sprint 7 from Sprint 6 (%v)", d, ids)
	}
}
