package main

import (
	"net/http"
	"testing"
	"time"
)

// A full sweep lists the project WITHOUT discussions and fetches them only for the
// issues that are new or changed since their link's last sync (#3105). On a big
// project the old single-phase sweep carried every thread of ~3000 issues and ran
// into GitLab's own GraphQL deadline; this pins the shape of the two phases.
func TestGitlabFullSweepTwoPhase(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-twophase-user", "grp-twophase")

	base := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	f.addIssue(glIssue{IID: 1, Title: "Busy", UpdatedAt: base, Notes: []glNote{
		{ID: 501, Body: "first", AuthorLogin: "someone", CreatedAt: base.Add(-time.Hour)},
	}})
	f.addIssue(glIssue{IID: 2, Title: "Quiet", UpdatedAt: base, Notes: []glNote{
		{ID: 502, Body: "only", AuthorLogin: "someone", CreatedAt: base.Add(-time.Hour)},
	}})

	connectGitlab(t, c, f)
	integID := createIntegration(t, c, s.WS, s.Board, f, nil)["id"].(string)

	// First sync — a full one, as the binding never had one. Both issues are new,
	// so both get their discussions.
	triggerSync(t, c, s.WS, integID)
	waitSyncRuns(t, c, s.WS, 1)
	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	busy, quiet := taskByIid(t, tasks, 1), taskByIid(t, tasks, 2)
	if n := len(c.get("/tasks/" + busy["id"].(string) + "/comments").listBody(t)); n != 1 {
		t.Fatalf("busy comments after first sweep = %d, want 1", n)
	}
	if n := len(c.get("/tasks/" + quiet["id"].(string) + "/comments").listBody(t)); n != 1 {
		t.Fatalf("quiet comments after first sweep = %d, want 1", n)
	}
	f.mu.Lock()
	first := f.issueQueries
	f.issueQueries = nil
	f.mu.Unlock()
	if len(first) < 2 || first[0].withNotes || first[0].iids != nil {
		t.Fatalf("first query = %+v, want a light list query", first)
	}
	if got := notedIIDs(first); len(got) != 2 || !got[1] || !got[2] {
		t.Fatalf("discussions fetched for %v, want both new issues", got)
	}

	// Issue 1 gets a comment and — as GitLab does — a new updatedAt. Issue 2 gets
	// a comment too, but keeps its updatedAt: the sweep reads it as unchanged and
	// pays nothing for its thread.
	f.mu.Lock()
	b := f.findIssueLocked(1)
	b.Notes = append(b.Notes, glNote{ID: 503, Body: "second", AuthorLogin: "someone", CreatedAt: base.Add(time.Minute)})
	b.UpdatedAt = base.Add(time.Minute)
	q := f.findIssueLocked(2)
	q.Notes = append(q.Notes, glNote{ID: 504, Body: "unseen", AuthorLogin: "someone", CreatedAt: base.Add(time.Minute)})
	f.mu.Unlock()

	r := c.post("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/sync?mode=full", nil)
	if r.Status != http.StatusAccepted {
		t.Fatalf("full sync: status %d\n%s", r.Status, r.Body)
	}
	runs := waitSyncRuns(t, c, s.WS, 2)
	if runs[0]["status"] != "ok" || runs[0]["mode"] != "full" {
		t.Fatalf("second run = %v, want an ok full sweep", runs[0])
	}
	if n := len(c.get("/tasks/" + busy["id"].(string) + "/comments").listBody(t)); n != 2 {
		t.Fatalf("busy comments after the sweep = %d, want 2", n)
	}
	if n := len(c.get("/tasks/" + quiet["id"].(string) + "/comments").listBody(t)); n != 1 {
		t.Fatalf("quiet comments after the sweep = %d, want 1 (unchanged issue, no thread fetched)", n)
	}
	f.mu.Lock()
	second := f.issueQueries
	f.mu.Unlock()
	if got := notedIIDs(second); len(got) != 1 || !got[1] {
		t.Fatalf("discussions fetched for %v, want only the changed issue #1", got)
	}
	for _, q := range second {
		if q.withNotes && q.iids == nil {
			t.Fatalf("a full sweep listed the project with discussions: %+v", q)
		}
	}
}

// notedIIDs collects the iids whose discussions a set of queries asked for.
func notedIIDs(qs []fakeIssueQuery) map[int64]bool {
	out := map[int64]bool{}
	for _, q := range qs {
		if !q.withNotes {
			continue
		}
		for _, iid := range q.iids {
			out[iid] = true
		}
	}
	return out
}

// findIssueLocked is findIssue for a caller already holding f.mu.
func (f *fakeGitlab) findIssueLocked(iid int64) *glIssue {
	for _, is := range f.issues {
		if is.IID == iid {
			return is
		}
	}
	return nil
}
