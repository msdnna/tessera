// Async-sync contract tests: a manual GitLab sync returns immediately, is
// journalled as "running" while it works, records a real duration, and reports
// its outcome to the user who started it via an `integration_sync` notification.
package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// runByID fetches one journal run out of the workspace list.
func runByID(t *testing.T, c *client, wsID, runID string) map[string]any {
	t.Helper()
	for _, r := range c.get("/workspaces/" + wsID + "/gitlab/sync-runs").listBody(t) {
		if r["id"] == runID {
			return r
		}
	}
	t.Fatalf("run %s not in journal", runID)
	return nil
}

// parseRunTime reads a timestamp out of a journal run row.
func parseRunTime(t *testing.T, run map[string]any, field string) time.Time {
	t.Helper()
	s, _ := run[field].(string)
	if s == "" {
		t.Fatalf("run %s is empty: %v", field, run)
	}
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("run %s = %q: %v", field, s, err)
	}
	return ts
}

// waitNotification polls the caller's feed for a notification of `kind`.
func waitNotification(t *testing.T, c *client, kind string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, n := range c.get("/notifications").listBody(t) {
			if n["kind"] == kind {
				return n
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no %q notification arrived", kind)
	return nil
}

// The manual sync is fire-and-forget: the request returns a run id straight
// away, the run is visible as in-flight, its recorded span covers the actual
// work (it used to be stamped start=finish=now at the very end, so every run
// read as instant), and the starter is notified when it lands.
func TestGitlabManualSyncRunsInBackground(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-async-user", "grp-async")
	f.addIssue(glIssue{IID: 1, Title: "Async issue"})
	// Several delayed round-trips per sync — long enough to observe the run
	// mid-flight without making the test slow.
	f.setDelay(80 * time.Millisecond)
	connectGitlab(t, c, f)

	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	runID := triggerSync(t, c, s.WS, integ["id"].(string))

	// The POST returned before the pull finished, and the journal already knows
	// about the run — that's what the UI watches instead of holding a request open.
	inflight := runByID(t, c, s.WS, runID)
	if inflight["status"] != "running" || inflight["finished_at"] != nil {
		t.Fatalf("run right after trigger: %v", inflight)
	}

	waitSyncRuns(t, c, s.WS, 1)
	done := runByID(t, c, s.WS, runID)
	if done["status"] != "ok" {
		t.Fatalf("finished run: %v", done)
	}
	startedAt := parseRunTime(t, done, "started_at")
	finishedAt := parseRunTime(t, done, "finished_at")
	// The span must cover the fake's latency, not collapse to zero.
	if d := finishedAt.Sub(startedAt); d < 80*time.Millisecond {
		t.Fatalf("run duration = %v, want at least the fake's per-request delay", d)
	}

	n := waitNotification(t, c, "integration_sync")
	text, _ := n["text"].(string)
	if !strings.Contains(text, f.projectPath) || !strings.Contains(text, "+1") {
		t.Fatalf("notification text = %q, want the project path and the created count", text)
	}
	// Not task-scoped — it reports on an integration, not on one card.
	if n["task_id"] != nil {
		t.Fatalf("integration_sync notification is task-scoped: %v", n)
	}
}

// A second manual sync while one is in flight is refused with already_running
// rather than queued — two overlapping pulls would fight over the same board.
func TestGitlabManualSyncRejectsOverlap(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-overlap-user", "grp-overlap")
	f.addIssue(glIssue{IID: 1, Title: "Overlap issue"})
	f.setDelay(80 * time.Millisecond)
	connectGitlab(t, c, f)

	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	integID := integ["id"].(string)
	triggerSync(t, c, s.WS, integID)

	second := c.expect(t, c.post("/workspaces/"+s.WS+"/gitlab/integrations/"+integID+"/sync", nil),
		http.StatusAccepted)
	if second["started"] != false || second["already_running"] != true {
		t.Fatalf("overlapping sync: %v", second)
	}
	// The refused call must not have opened a second journal row.
	waitSyncRuns(t, c, s.WS, 1)
	pulls := 0
	for _, r := range c.get("/workspaces/" + s.WS + "/gitlab/sync-runs").listBody(t) {
		if r["kind"] == "pull" {
			pulls++
		}
	}
	if pulls != 1 {
		t.Fatalf("journal has %d pull runs after a refused overlap, want 1", pulls)
	}
}

// A crash mid-sync leaves a run stuck in "running"; startup closes those out so
// the journal doesn't show a phantom sync spinning forever.
//
// Deliberately NOT parallel: FailStaleSyncRuns is process-wide, so it must not
// run while another test's sync is in flight.
func TestGitlabFailStaleSyncRuns(t *testing.T) {
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-stale-user", "grp-stale")
	connectGitlab(t, c, f)

	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	ctx := context.Background()
	// Stand in for a run whose process died before it could finish.
	stale, err := testQueries.CreateGitlabSyncRun(ctx, db.CreateGitlabSyncRunParams{
		IntegrationID: uuid.MustParse(integ["id"].(string)),
		Kind:          "pull", Trigger: "manual", Status: "running",
		StartedAt: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed stale run: %v", err)
	}

	testAPI.FailStaleSyncRuns(ctx)

	got := runByID(t, c, s.WS, stale.ID.String())
	if got["status"] != "error" || got["finished_at"] == nil {
		t.Fatalf("stale run after cleanup: %v", got)
	}
	if txt, _ := got["error"].(string); txt == "" {
		t.Fatalf("stale run has no error reason: %v", got)
	}
}
