package main

import (
	"context"
	"net/http"
	"testing"
)

// A long full sweep must stay in the background-jobs panel once it finishes.
// The panel drops finished syncs from the live in-memory registry and expects the
// durable journal to back them, but the journal window filtered on started_at —
// so a sweep that ran longer than the window (JOBS_JOURNAL_TTL, 1h by default)
// fell out of it at the exact moment it completed and the row simply vanished.
// Short incremental runs always finished inside the window, which is why only
// full syncs disappeared (#2751).
func TestAdminJobsKeepsLongFullSyncRun(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-journal-window", "grp-journal-window")
	connectGitlab(t, c, f)

	integ := createIntegration(t, c, s.WS, s.Board, f, nil)

	// Seed the journal directly rather than timing a real pull: the subject here
	// is the retention window, and a 90-minute sweep can only be expressed by
	// writing the timestamps. It started outside the default 1h window and
	// finished a minute ago, well inside it.
	var runID string
	if err := testPool.QueryRow(context.Background(),
		`INSERT INTO gitlab_sync_runs
		     (integration_id, kind, trigger, mode, status, started_at, finished_at)
		 VALUES ($1, 'pull', 'auto', 'full', 'ok',
		         now() - interval '90 minutes', now() - interval '1 minute')
		 RETURNING id`, integ["id"].(string)).Scan(&runID); err != nil {
		t.Fatalf("seed journal run: %v", err)
	}

	var run map[string]any
	for _, j := range c.get("/admin/jobs").listBody(t) {
		if j["key"] == "syncrun:"+runID {
			run = j
		}
	}
	if run == nil {
		t.Fatalf("full sync run %s missing from /admin/jobs — it finished inside the window", runID)
	}
	if run["status"] != "done" {
		t.Errorf("run status = %v, want done", run["status"])
	}
	if run["mode"] != "full" {
		t.Errorf("run mode = %v, want full (the panel labels the sweep by it)", run["mode"])
	}
	if run["persisted"] != true {
		t.Errorf("run persisted = %v, want true (it comes from the journal)", run["persisted"])
	}
}

func TestAdminJobsAPI(t *testing.T) {
	c := signup(t)

	// The very first registered user is auto-promoted to admin, so pin this one to
	// non-admin to exercise the 403 path deterministically regardless of test order.
	if _, err := testPool.Exec(context.Background(),
		`UPDATE users SET is_admin = false WHERE id = $1`, c.UserID); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if r := c.get("/admin/jobs"); r.Status != http.StatusForbidden {
		t.Fatalf("non-admin GET /admin/jobs = %d, want 403", r.Status)
	}

	makeAdmin(t, c)

	// The registry lists the tick-loop workers registered at startup.
	jobs := c.get("/admin/jobs").listBody(t)
	byKey := map[string]map[string]any{}
	for _, j := range jobs {
		byKey[j["key"].(string)] = j
	}
	for _, key := range []string{"gitlab_sync_cron", "gitlab_writeback", "notify_delivery", "notify_scanner", "recurrence"} {
		w, ok := byKey[key]
		if !ok {
			t.Fatalf("worker %q missing from /admin/jobs", key)
		}
		if w["kind"] != "worker" || w["status"] != "running" {
			t.Fatalf("worker %q wrong: %v", key, w)
		}
		if iv, _ := w["interval_sec"].(float64); iv <= 0 {
			t.Fatalf("worker %q missing interval_sec (for next-run estimate): %v", key, w)
		}
	}

	// A known worker can be run on demand (detached) → 202.
	if r := c.post("/admin/jobs/notify_scanner/run", nil); r.Status != http.StatusAccepted {
		t.Fatalf("run notify_scanner = %d, want 202\n%s", r.Status, r.Body)
	}
	// An unknown job is not runnable → 400.
	if r := c.post("/admin/jobs/nope/run", nil); r.Status != http.StatusBadRequest {
		t.Fatalf("run unknown = %d, want 400", r.Status)
	}
	// A worker is not cancelable (no cancel func) → 409.
	if r := c.post("/admin/jobs/notify_scanner/cancel", nil); r.Status != http.StatusConflict {
		t.Fatalf("cancel worker = %d, want 409", r.Status)
	}
}
