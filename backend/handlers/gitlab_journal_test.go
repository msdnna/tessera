package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// TestFmtSyncDuration covers the short-unit rendering used in the "sync finished"
// notification — a background run's only visible cost.
func TestFmtSyncDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "меньше секунды"},
		{300 * time.Millisecond, "меньше секунды"},
		{900 * time.Millisecond, "1 с"},
		{12 * time.Second, "12 с"},
		{59 * time.Second, "59 с"},
		{72 * time.Second, "1 м 12 с"},
		{5 * time.Minute, "5 м 0 с"},
		{63 * time.Minute, "1 ч 3 м"},
	}
	for _, tc := range cases {
		if got := fmtSyncDuration(tc.in); got != tc.want {
			t.Errorf("fmtSyncDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestNotifySyncFinishedSkipsSilentRuns pins the guards on who gets told. The API
// carries no queries here, so any attempt to actually deliver would panic —
// which is exactly the assertion: these runs must not reach the DB at all.
//
// An auto run must stay silent (a 5-minute interval would otherwise spam the
// bell), and a run with no real actor has nobody to tell.
func TestNotifySyncFinishedSkipsSilentRuns(t *testing.T) {
	h := &API{}
	integ := db.GitlabIntegration{ID: uuid.New(), WorkspaceID: uuid.New(), ProjectPath: "grp/proj"}
	actor := uuid.New()
	nilActor := uuid.Nil

	cases := []struct {
		name string
		j    *syncJournal
	}{
		{"nil journal", nil},
		{"auto run", &syncJournal{trigger: "auto", actorID: &actor, status: "ok"}},
		{"manual run without an actor", &syncJournal{trigger: "manual", status: "ok"}},
		{"manual run with a nil actor", &syncJournal{trigger: "manual", actorID: &nilActor, status: "ok"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(_ *testing.T) {
			h.notifySyncFinished(context.Background(), integ, tc.j, 1, 2, nil)
		})
	}
}

// TestNewJournalStampsStart pins that a journal carries the moment the run began
// — the run row used to be stamped started_at=finished_at=now() at flush time, so
// every run in the UI read as instant.
func TestNewJournalStampsStart(t *testing.T) {
	h := &API{}
	before := time.Now()
	j := h.newJournal(uuid.New(), "pull", "manual", nil)
	if j.startedAt.Before(before) || time.Since(j.startedAt) > time.Minute {
		t.Fatalf("startedAt = %v, want ~now", j.startedAt)
	}
	if j.runID != nil {
		t.Fatalf("a fresh journal must not claim a run row: %v", j.runID)
	}
	if j.status != "ok" {
		t.Fatalf("fresh journal status = %q, want ok", j.status)
	}
}
