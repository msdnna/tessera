package main

import (
	"context"
	"net/http"
	"testing"
)

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
