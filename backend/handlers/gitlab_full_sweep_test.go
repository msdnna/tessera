package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// A periodic full sweep that failed (GitLab timing out on a big project) used to
// be retried every tick as "full" again, so even the cheap incremental pull never
// ran and the board stopped updating. A failed sweep now yields to incremental
// pulls for fullSweepRetryAfter.
func TestFullSweepBackingOff(t *testing.T) {
	now := time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)
	lastFull := now.Add(-48 * time.Hour)
	integ := db.GitlabIntegration{ID: uuid.New(), LastFullSyncedAt: &lastFull, FullSyncIntervalSec: 86400}
	t.Cleanup(func() { fullSweepFailedAt.Delete(integ.ID) })

	if fullSweepBackingOff(integ, now) {
		t.Fatal("no failure recorded — the sweep must not be held back")
	}

	fullSweepFailedAt.Store(integ.ID, now.Add(-10*time.Minute))
	if !fullSweepBackingOff(integ, now) {
		t.Fatal("a sweep that failed 10 minutes ago must yield to an incremental pull")
	}
	if fullSweepBackingOff(integ, now.Add(fullSweepRetryAfter)) {
		t.Fatal("after fullSweepRetryAfter the sweep is tried again")
	}

	// Never fully synced: there is no delta to pull, so the full sync always runs.
	fresh := db.GitlabIntegration{ID: uuid.New()}
	t.Cleanup(func() { fullSweepFailedAt.Delete(fresh.ID) })
	fullSweepFailedAt.Store(fresh.ID, now.Add(-time.Minute))
	if fullSweepBackingOff(fresh, now) {
		t.Fatal("a binding without a full sync must never be held back")
	}
}
