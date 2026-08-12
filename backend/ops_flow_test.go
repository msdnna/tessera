package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// The readiness probe pings the DB; against the live test database it must report
// healthy with a db block.
func TestReadyProbeOK(t *testing.T) {
	r := doReq(t, "", http.MethodGet, "/health/ready", nil)
	if r.Status != http.StatusOK {
		t.Fatalf("ready status = %d, want 200\n%s", r.Status, r.Body)
	}
	m := r.mapBody(t)
	if m["ok"] != true {
		t.Fatalf("ready ok = %v, want true", m["ok"])
	}
	dbBlock, ok := m["db"].(map[string]any)
	if !ok || dbBlock["ok"] != true {
		t.Fatalf("ready db block = %v, want ok:true", m["db"])
	}
}

// /admin/metrics is admin-only and reports the process internals.
func TestMetricsRequiresAdmin(t *testing.T) {
	c := signup(t)
	uid := uuid.MustParse(c.UserID)

	// The very first user in a run is auto-admin; pin this one to non-admin so the
	// gate check is deterministic regardless of test order.
	if err := testQueries.SetUserAdmin(context.Background(), db.SetUserAdminParams{ID: uid, IsAdmin: false}); err != nil {
		t.Fatalf("demote: %v", err)
	}
	if r := c.get("/admin/metrics"); r.Status != http.StatusForbidden {
		t.Fatalf("non-admin metrics status = %d, want 403\n%s", r.Status, r.Body)
	}

	// Elevate this user to admin and retry.
	if err := testQueries.SetUserAdmin(context.Background(), db.SetUserAdminParams{
		ID: uuid.MustParse(c.UserID), IsAdmin: true,
	}); err != nil {
		t.Fatalf("elevate to admin: %v", err)
	}

	r := c.get("/admin/metrics")
	if r.Status != http.StatusOK {
		t.Fatalf("admin metrics status = %d, want 200\n%s", r.Status, r.Body)
	}
	m := r.mapBody(t)
	for _, k := range []string{"db_pool", "ws_clients", "jobs", "http", "version"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("metrics body missing %q: %v", k, m)
		}
	}
}
