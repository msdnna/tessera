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

// TestSkipEmptyRun pins which runs may go unrecorded. The rule exists to keep the
// every-few-minutes auto heartbeat out of the journal, but it used to swallow the
// daily full sweep too — and since the background-jobs panel drops finished syncs
// from the live registry and expects the journal to back them, an unchanged full
// sweep simply vanished from the panel the moment it finished (#2751).
func TestSkipEmptyRun(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		trigger string
		mode    string
		actions int
		status  string
		want    bool
	}{
		{"auto incremental pull, no changes", "pull", "auto", "incremental", 0, "ok", true},
		{"auto full sweep, no changes", "pull", "auto", "full", 0, "ok", false},
		{"auto incremental pull with changes", "pull", "auto", "incremental", 2, "ok", false},
		{"auto full sweep with changes", "pull", "auto", "full", 2, "ok", false},
		{"manual incremental pull, no changes", "pull", "manual", "incremental", 0, "ok", false},
		{"manual full sweep, no changes", "pull", "manual", "full", 0, "ok", false},
		{"failed auto incremental pull", "pull", "auto", "incremental", 0, "error", false},
		{"auto push, nothing delivered", "push", "auto", "full", 0, "ok", true},
		{"auto push with deliveries", "push", "auto", "full", 1, "ok", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := &syncJournal{
				kind: tc.kind, trigger: tc.trigger, mode: tc.mode, status: tc.status,
				actions: make([]journalAction, tc.actions),
			}
			if got := j.skipEmpty(); got != tc.want {
				t.Errorf("skipEmpty() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCreateParamsCarriesMode pins that the run row records how the pull fetched
// its issues. beginJournal and flushJournal used to build these params separately
// and the flush copy left Mode unset, so every run created at flush time (i.e.
// every auto run) landed with a blank mode and the panel could not tell a full
// sweep from an incremental one.
func TestCreateParamsCarriesMode(t *testing.T) {
	integID, actorID := uuid.New(), uuid.New()
	started := time.Now().Add(-90 * time.Minute)
	j := &syncJournal{
		integrationID: integID, kind: "pull", trigger: "auto", mode: "full",
		actorID: &actorID, startedAt: started, status: "ok",
	}
	p := j.createParams()
	if p.Mode != "full" {
		t.Errorf("Mode = %q, want full", p.Mode)
	}
	if p.IntegrationID != integID || p.Kind != "pull" || p.Trigger != "auto" {
		t.Errorf("params lost identity: %+v", p)
	}
	if p.ActorID == nil || *p.ActorID != actorID {
		t.Errorf("ActorID = %v, want %v", p.ActorID, actorID)
	}
	if !p.StartedAt.Equal(started) {
		t.Errorf("StartedAt = %v, want the real start %v", p.StartedAt, started)
	}
	if p.Status != "running" {
		t.Errorf("Status = %q, want running (FinishGitlabSyncRun stamps the outcome)", p.Status)
	}
}
