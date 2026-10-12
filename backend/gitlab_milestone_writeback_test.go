// GitLab stages both ways (#3010): a stage linked to a GitLab milestone is edited
// and closed in Tessera and written to GitLab, compared three ways against the
// last sync — a change made in both places is parked as a conflict for the
// conflicts window, a failed write stays «not synced» until a retry, and a
// binding that does not write keeps the stage read-only.
package main

import (
	"context"
	"net/http"
	"slices"
	"testing"
)

// gitlabStage sets up a binding over a fake project with one issue on
// milestone 41 ("Sprint 1"), runs the first pull and returns the native stage.
type gitlabStage struct {
	c       *client
	s       stack
	f       *fakeGitlab
	integID string
	id      string
	runs    int
}

func newGitlabStage(t *testing.T, user string, integExtra map[string]any) *gitlabStage {
	t.Helper()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, user, "grp-"+user)
	f.addIssue(glIssue{
		IID: 1, Title: "Ship login",
		Milestone: &glMilestone{
			ID: 41, IID: 4, Title: "Sprint 1", Description: "Goals: ship login", State: "active",
			StartDate: "2026-08-01", DueDate: "2026-08-31",
		},
	})
	connectGitlab(t, c, f)
	integID := createIntegration(t, c, s.WS, s.Board, f, integExtra)["id"].(string)
	g := &gitlabStage{c: c, s: s, f: f, integID: integID}
	g.sync(t)
	g.id = g.stage(t)["id"].(string)
	return g
}

// sync runs a full pull (a milestone edit does not touch the issue's
// updatedAt for an incremental one) and waits for it.
func (g *gitlabStage) sync(t *testing.T) {
	t.Helper()
	if r := g.c.post("/workspaces/"+g.s.WS+"/gitlab/integrations/"+g.integID+"/sync?mode=full", nil); r.Status != http.StatusAccepted {
		t.Fatalf("sync: status %d\n%s", r.Status, r.Body)
	}
	g.runs++
	waitSyncRuns(t, g.c, g.s.WS, g.runs)
}

// stage reads the stage as the project list serves it.
func (g *gitlabStage) stage(t *testing.T) map[string]any {
	t.Helper()
	for _, m := range g.c.get("/projects/" + g.s.Project + "/milestones").listBody(t) {
		if m["gl_global_id"] == "gid://gitlab/Milestone/41" {
			return m
		}
	}
	t.Fatalf("GitLab stage was not mirrored")
	return nil
}

// patch edits the stage: the current values with the given overrides.
func (g *gitlabStage) patch(t *testing.T, over map[string]any) map[string]any {
	t.Helper()
	m := g.stage(t)
	body := map[string]any{
		"title": m["title"], "description": m["description"],
		"start_date": m["start_date"], "due_date": m["due_date"], "state": m["state"],
	}
	for k, v := range over {
		body[k] = v
	}
	return g.c.expect(t, g.c.patch("/milestones/"+g.id, body), http.StatusOK)
}

func syncStatus(t *testing.T, res map[string]any) string {
	t.Helper()
	gs, _ := res["gl_sync"].(map[string]any)
	if gs == nil {
		t.Fatalf("response has no gl_sync: %v", res)
	}
	s, _ := gs["status"].(string)
	return s
}

func writableBinding() map[string]any {
	return map[string]any{"writeback": map[string]any{"enabled": true}}
}

// Goals, dates and state are written to GitLab; what GitLab changes alone comes
// back with the pull; the stage stays as written across later pulls.
func TestGitlabStageWriteback_EditDatesAndClose(t *testing.T) {
	t.Parallel()
	g := newGitlabStage(t, "gl-stw-edit", writableBinding())
	if m := g.stage(t); m["gl_writable"] != true || m["gl_push_error"] != "" || m["gl_conflict"] != false {
		t.Fatalf("stage write-back state: %v", m)
	}

	res := g.patch(t, map[string]any{
		"description": "## Goals\n- ship login\n- ship signup",
		"due_date":    "2026-09-05T00:00:00Z",
	})
	if st := syncStatus(t, res); st != "ok" {
		t.Fatalf("edit gl_sync = %v", res["gl_sync"])
	}
	remote := g.f.milestone(41)
	if remote.Description != "## Goals\n- ship login\n- ship signup" || remote.DueDate != "2026-09-05" {
		t.Fatalf("GitLab milestone after edit: %+v", remote)
	}
	// Only what changed went out.
	g.f.mu.Lock()
	put := g.f.milestonePuts[len(g.f.milestonePuts)-1]
	g.f.mu.Unlock()
	if _, has := put["title"]; has || put.Get("due_date") != "2026-09-05" {
		t.Fatalf("PUT form = %v, want description + due_date only", put)
	}

	// Close (⇧X) goes as state_event=close; closed_at follows.
	res = g.patch(t, map[string]any{"state": "closed"})
	if st := syncStatus(t, res); st != "ok" || res["closed_at"] == nil {
		t.Fatalf("close: %v", res)
	}
	if remote := g.f.milestone(41); remote.State != "closed" {
		t.Fatalf("GitLab state after close = %q", remote.State)
	}

	// A pull with nothing new changes nothing.
	g.sync(t)
	if m := g.stage(t); m["state"] != "closed" || m["description"] != "## Goals\n- ship login\n- ship signup" {
		t.Fatalf("stage after quiet pull: %v", m)
	}

	// GitLab reopens and retitles it: the pull takes both.
	g.f.editMilestone(41, func(m *glMilestone) {
		m.State = "active"
		m.Title = "Sprint 1 (extended)"
	})
	g.sync(t)
	if m := g.stage(t); m["state"] != "active" || m["title"] != "Sprint 1 (extended)" || m["closed_at"] != nil {
		t.Fatalf("stage after GitLab reopen: %v", m)
	}
}

// A binding that does not write to GitLab — write-back off, or the binding
// itself off — keeps the stage read-only, and nothing reaches GitLab.
func TestGitlabStageWriteback_ReadOnlyBindingDoesNotWrite(t *testing.T) {
	t.Parallel()
	g := newGitlabStage(t, "gl-stw-ro", nil)
	if m := g.stage(t); m["gl_writable"] != false {
		t.Fatalf("gl_writable without write-back = %v", m["gl_writable"])
	}
	r := g.c.patch("/milestones/"+g.id, map[string]any{"title": "Sprint 1", "description": "local", "state": "active"})
	if r.Status != http.StatusConflict || r.mapBody(t)["code"] != "gitlab_readonly" {
		t.Fatalf("edit with write-back off: status %d\n%s", r.Status, r.Body)
	}
	// Write-back on, binding off: still read-only.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE gitlab_integrations SET enabled = false, writeback = '{"enabled": true}'::jsonb WHERE id = $1::uuid`,
		g.integID); err != nil {
		t.Fatalf("switch binding off: %v", err)
	}
	r = g.c.patch("/milestones/"+g.id, map[string]any{"title": "Sprint 1", "description": "local", "state": "closed"})
	if r.Status != http.StatusConflict {
		t.Fatalf("edit with binding off: status %d\n%s", r.Status, r.Body)
	}
	if r := g.c.post("/milestones/"+g.id+"/gitlab/sync", nil); r.Status != http.StatusConflict {
		t.Fatalf("retry with binding off: status %d", r.Status)
	}
	g.f.mu.Lock()
	puts := len(g.f.milestonePuts)
	g.f.mu.Unlock()
	if puts != 0 {
		t.Fatalf("GitLab got %d milestone writes from a read-only binding", puts)
	}
	if m := g.stage(t); m["description"] != "Goals: ship login" || m["state"] != "active" {
		t.Fatalf("read-only stage changed: %v", m)
	}
}

// The goals changed in both places: the write parks a conflict instead of
// overwriting GitLab, the conflicts window lists it (opt-in), and each of the
// three resolutions settles it.
func TestGitlabStageWriteback_DescriptionConflict(t *testing.T) {
	t.Parallel()
	g := newGitlabStage(t, "gl-stw-conf", writableBinding())

	conflicts := func(query string) []map[string]any {
		t.Helper()
		var out []map[string]any
		for _, c := range g.c.get("/workspaces/" + g.s.WS + "/gitlab/conflicts" + query).listBody(t) {
			if c["entity"] == "milestone" {
				out = append(out, c)
			}
		}
		return out
	}
	diverge := func(gitlab, tessera string) {
		t.Helper()
		g.f.editMilestone(41, func(m *glMilestone) { m.Description = gitlab })
		res := g.patch(t, map[string]any{"description": tessera})
		if st := syncStatus(t, res); st != "conflict" {
			t.Fatalf("diverged edit gl_sync = %v", res["gl_sync"])
		}
		fields, _ := res["gl_sync"].(map[string]any)["fields"].([]any)
		if len(fields) != 1 || fields[0] != "description" {
			t.Fatalf("conflict fields = %v", fields)
		}
	}

	diverge("Goals from GitLab", "Goals from Tessera")
	if got := g.f.milestone(41).Description; got != "Goals from GitLab" {
		t.Fatalf("GitLab overwritten despite the conflict: %q", got)
	}
	if m := g.stage(t); m["description"] != "Goals from Tessera" || m["gl_conflict"] != true {
		t.Fatalf("stage while in conflict: %v", m)
	}
	if n := len(conflicts("")); n != 0 {
		t.Fatalf("stage conflict listed without include=milestones (%d)", n)
	}
	list := conflicts("?include=milestones")
	if len(list) != 1 || list[0]["milestone_id"] != g.id || list[0]["task_id"] != nil ||
		list[0]["milestone_title"] != "Sprint 1" || list[0]["change_kind"] != "milestone" {
		t.Fatalf("conflict list = %v", list)
	}
	f0 := list[0]["fields"].([]any)[0].(map[string]any)
	if f0["field"] != "description" || f0["base"] != "Goals: ship login" ||
		f0["ours"] != "Goals from Tessera" || f0["theirs"] != "Goals from GitLab" {
		t.Fatalf("conflict field = %v", f0)
	}
	// A pull keeps the local text while the conflict waits.
	g.sync(t)
	if m := g.stage(t); m["description"] != "Goals from Tessera" || m["gl_conflict"] != true {
		t.Fatalf("pull clobbered a conflicted stage: %v", m)
	}

	// manual: the merged text goes to both.
	res := g.c.expect(t, g.c.post("/milestones/"+g.id+"/gitlab/conflict/resolve", map[string]any{
		"resolution": "manual", "value": map[string]any{"description": "Merged goals"},
	}), http.StatusOK)
	if gs, _ := res["gl_sync"].(map[string]any); gs["status"] != "ok" {
		t.Fatalf("manual resolve gl_sync = %v", res["gl_sync"])
	}
	if got := g.f.milestone(41).Description; got != "Merged goals" {
		t.Fatalf("GitLab after manual = %q", got)
	}
	if n := len(conflicts("?include=milestones")); n != 0 {
		t.Fatalf("conflict still open after manual (%d)", n)
	}

	// theirs: GitLab's text comes in, nothing is written.
	diverge("GitLab wins", "Tessera loses")
	g.f.mu.Lock()
	before := len(g.f.milestonePuts)
	g.f.mu.Unlock()
	g.c.expect(t, g.c.post("/milestones/"+g.id+"/gitlab/conflict/resolve", map[string]any{"resolution": "theirs"}), http.StatusOK)
	g.f.mu.Lock()
	after := len(g.f.milestonePuts)
	g.f.mu.Unlock()
	if after != before {
		t.Fatalf("theirs wrote to GitLab (%d → %d)", before, after)
	}
	if m := g.stage(t); m["description"] != "GitLab wins" || m["gl_conflict"] != false {
		t.Fatalf("stage after theirs: %v", m)
	}

	// ours: Tessera's text overwrites GitLab.
	diverge("GitLab loses", "Tessera wins")
	g.c.expect(t, g.c.post("/milestones/"+g.id+"/gitlab/conflict/resolve", map[string]any{"resolution": "ours"}), http.StatusOK)
	if got := g.f.milestone(41).Description; got != "Tessera wins" {
		t.Fatalf("GitLab after ours = %q", got)
	}
	// Nothing left to resolve.
	if r := g.c.post("/milestones/"+g.id+"/gitlab/conflict/resolve", map[string]any{"resolution": "ours"}); r.Status != http.StatusNotFound {
		t.Fatalf("resolve without a conflict: status %d", r.Status)
	}
}

// GitLab refuses the write: the stage keeps the edit, marked «not synced»; a
// pull does not undo it; «retry» sends it, and so does the next sync by itself.
func TestGitlabStageWriteback_FailedWriteRetries(t *testing.T) {
	t.Parallel()
	g := newGitlabStage(t, "gl-stw-fail", writableBinding())

	g.f.mu.Lock()
	g.f.milestonePutStatus = http.StatusInternalServerError
	g.f.mu.Unlock()
	res := g.patch(t, map[string]any{"description": "Unsent goals"})
	if st := syncStatus(t, res); st != "failed" {
		t.Fatalf("refused write gl_sync = %v", res["gl_sync"])
	}
	m := g.stage(t)
	fields, _ := m["gl_push_fields"].([]any)
	if m["description"] != "Unsent goals" || m["gl_push_error"] == "" || !slices.Contains(fields, any("description")) {
		t.Fatalf("stage after refused write: %v", m)
	}
	g.sync(t) // the retry inside fails too; the pull must keep the edit
	if m := g.stage(t); m["description"] != "Unsent goals" || m["gl_push_error"] == "" {
		t.Fatalf("pull clobbered an unsent edit: %v", m)
	}

	// «retry» once GitLab is back.
	g.f.mu.Lock()
	g.f.milestonePutStatus = 0
	g.f.mu.Unlock()
	res = g.c.expect(t, g.c.post("/milestones/"+g.id+"/gitlab/sync", nil), http.StatusOK)
	if st := syncStatus(t, res); st != "ok" {
		t.Fatalf("retry gl_sync = %v", res["gl_sync"])
	}
	if got := g.f.milestone(41).Description; got != "Unsent goals" {
		t.Fatalf("GitLab after retry = %q", got)
	}
	if m := g.stage(t); m["gl_push_error"] != "" {
		t.Fatalf("not-synced mark left after retry: %v", m)
	}

	// The sync retries by itself.
	g.f.mu.Lock()
	g.f.milestonePutStatus = http.StatusBadGateway
	g.f.mu.Unlock()
	if st := syncStatus(t, g.patch(t, map[string]any{"title": "Sprint 1 · renamed"})); st != "failed" {
		t.Fatalf("second refused write did not fail")
	}
	g.f.mu.Lock()
	g.f.milestonePutStatus = 0
	g.f.mu.Unlock()
	g.sync(t)
	if got := g.f.milestone(41).Title; got != "Sprint 1 · renamed" {
		t.Fatalf("sync did not retry the write: GitLab title %q", got)
	}
	if m := g.stage(t); m["gl_push_error"] != "" || m["title"] != "Sprint 1 · renamed" {
		t.Fatalf("stage after the sync's retry: %v", m)
	}
}

// A link from before the baseline existed (no gl_snapshot): the first edit
// takes the stage's pre-edit values as the baseline, so GitLab's untouched
// fields are not mistaken for a conflict.
func TestGitlabStageWriteback_LegacyLinkWithoutBaseline(t *testing.T) {
	t.Parallel()
	g := newGitlabStage(t, "gl-stw-legacy", writableBinding())
	if _, err := testPool.Exec(context.Background(),
		`UPDATE gitlab_milestone_links SET gl_snapshot = '{}'::jsonb WHERE milestone_id = $1::uuid`, g.id); err != nil {
		t.Fatalf("drop baseline: %v", err)
	}
	if st := syncStatus(t, g.patch(t, map[string]any{"description": "First edit"})); st != "ok" {
		t.Fatalf("edit on a legacy link: not ok")
	}
	if got := g.f.milestone(41).Description; got != "First edit" {
		t.Fatalf("GitLab after legacy edit = %q", got)
	}
}

// Write-back switched off while an edit was still unsent: nothing local can go
// out any more, so the pull hands the stage back to GitLab and drops the mark.
func TestGitlabStageWriteback_WritebackOffHandsStageBack(t *testing.T) {
	t.Parallel()
	g := newGitlabStage(t, "gl-stw-off", writableBinding())
	g.f.mu.Lock()
	g.f.milestonePutStatus = http.StatusInternalServerError
	g.f.mu.Unlock()
	if st := syncStatus(t, g.patch(t, map[string]any{"description": "Never sent"})); st != "failed" {
		t.Fatalf("refused write did not fail")
	}
	if _, err := testPool.Exec(context.Background(),
		`UPDATE gitlab_integrations SET writeback = '{}'::jsonb WHERE id = $1::uuid`, g.integID); err != nil {
		t.Fatalf("switch write-back off: %v", err)
	}
	g.sync(t)
	if m := g.stage(t); m["description"] != "Goals: ship login" || m["gl_push_error"] != "" || m["gl_writable"] != false {
		t.Fatalf("stage after write-back went off: %v", m)
	}
}
