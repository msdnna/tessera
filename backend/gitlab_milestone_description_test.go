// A GitLab milestone is mirrored as a native project milestone, and GitLab is
// the source of truth for its synced fields. The description used to be left
// out of the pull — neither queried nor written — so every sync reset the native
// milestone's description to "", wiping even the one Tessera had pushed (#2968).
package main

import (
	"net/http"
	"testing"
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
