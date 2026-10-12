package main

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// A long run must stay in the background-jobs window once it finishes. The window
// drops finished runs from the live in-memory registry and expects the durable
// history to back them; the history window used to filter on a short TTL from the
// start, so a sweep that ran longer than the window fell out of it at the exact
// moment it completed and the row simply vanished (#2751). Since #3067 the
// history is job_runs and the window is its 30-day retention.
func TestAdminJobsKeepsLongFullSyncRun(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)

	// Seed the history directly: the subject is the retention window, and a
	// 90-minute sweep can only be expressed by writing the timestamps.
	var runID string
	if err := testPool.QueryRow(context.Background(),
		`INSERT INTO job_runs (id, job_key, kind, type, name, name_key, name_arg, trigger, status, meta,
		                       queued_at, started_at, finished_at)
		 VALUES (gen_random_uuid(), 'gitlab_sync:x', 'sync', 'gitlab', 'Синхронизация GitLab · x', 'gitlab_sync', 'x',
		         'auto', 'done', '{"mode":"full"}',
		         now() - interval '90 minutes', now() - interval '90 minutes', now() - interval '1 minute')
		 RETURNING id`).Scan(&runID); err != nil {
		t.Fatalf("seed history run: %v", err)
	}

	var run map[string]any
	for _, j := range c.get("/admin/jobs").listBody(t) {
		if j["run_id"] == runID {
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
		t.Errorf("run mode = %v, want full (the window labels the sweep by it)", run["mode"])
	}
	if run["persisted"] != true {
		t.Errorf("run persisted = %v, want true (it comes from the history)", run["persisted"])
	}
	// The type filter narrows the list to one family.
	for _, j := range c.get("/admin/jobs?type=import_export").listBody(t) {
		if j["run_id"] == runID {
			t.Fatalf("type=import_export returned a gitlab run")
		}
	}
	// One run reads back by its history key.
	one := c.expect(t, c.get("/admin/jobs/jobrun:"+runID), http.StatusOK)
	if one["run_id"] != runID {
		t.Fatalf("GET one run = %v", one)
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
		if w["type"] == "" || w["type"] == nil {
			t.Fatalf("worker %q has no type for the window's filter: %v", key, w)
		}
	}

	// A known worker can be run on demand (detached) → 202, and the pass is a run of
	// its own in the history, credited to whoever pressed the button (#3067).
	r := c.post("/admin/jobs/notify_scanner/run", nil)
	if r.Status != http.StatusAccepted {
		t.Fatalf("run notify_scanner = %d, want 202\n%s", r.Status, r.Body)
	}
	runID, _ := r.mapBody(t)["run_id"].(string)
	if runID == "" {
		t.Fatalf("run-now answered without a run_id: %s", r.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		one := c.get("/admin/jobs/jobrun:" + runID)
		if one.Status == http.StatusOK {
			m := one.mapBody(t)
			if m["status"] == "done" {
				if m["kind"] != "run" || m["trigger"] != "manual" || m["actor_id"] != c.UserID || m["type"] != "notify" {
					t.Fatalf("run-now history row wrong: %v", m)
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("run-now never finished: %d %s", one.Status, one.Body)
		}
		time.Sleep(50 * time.Millisecond)
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
