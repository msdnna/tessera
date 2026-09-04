package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tessera/internal/db"
)

// The recording store (#2864, subtask #2877) is exercised at the SQL level
// rather than through HTTP: the handlers arrive in the next commit, and the
// guarantees below live in the schema, not in Go. Each of them replaces a check
// the handler would otherwise have to make — and would race on.

// mkRecording starts an in-flight recording row for a conference.
func mkRecording(t *testing.T, confID uuid.UUID, egressID string, by *uuid.UUID) (db.ConferenceRecording, error) {
	t.Helper()
	return testQueries.CreateConferenceRecording(context.Background(), db.CreateConferenceRecordingParams{
		ConferenceID: confID,
		EgressID:     egressID,
		FilePath:     "conf/" + confID.String() + "/rec-" + egressID + ".mp4",
		FileName:     "Летучка.mp4",
		StartedBy:    by,
	})
}

// uniqueViolation reports Postgres's 23505, the way the handlers will have to
// read it to turn a lost race into "recording is already running" instead of a
// 500.
func uniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func mustUUID(t *testing.T, s string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", s, err)
	}
	return id
}

// TestConferenceRecordingOneActive covers the partial unique index that makes
// "one recording at a time" true rather than merely checked.
//
// The handler will look for a live recording before starting one, and two hosts
// pressing record in the same second would both find none. Without the index the
// second egress worker would write a file no row points at: never listed, never
// swept, never deleted.
func TestConferenceRecordingOneActive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := signup(t)
	_, confIDs := mkConference(t, owner, "Летучка")
	confID := mustUUID(t, confIDs)
	by := mustUUID(t, owner.UserID)

	first, err := mkRecording(t, confID, "EG_"+uuid.NewString(), &by)
	if err != nil {
		t.Fatalf("first recording: %v", err)
	}
	if first.Status != "active" {
		t.Fatalf("a fresh recording is %q, want active", first.Status)
	}

	if _, err := mkRecording(t, confID, "EG_"+uuid.NewString(), &by); !uniqueViolation(err) {
		t.Fatalf("second concurrent recording was accepted (err=%v), want a unique violation", err)
	}

	// Once the first one is over the slot frees up — a conference gets recorded
	// again tomorrow, and the index must not turn into "one recording ever".
	if _, err := testQueries.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
		ID: first.ID, Status: "completed", SizeBytes: 1024, DurationSec: 42,
	}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := mkRecording(t, confID, "EG_"+uuid.NewString(), &by); err != nil {
		t.Fatalf("recording after the previous one finished: %v", err)
	}
}

// TestConferenceRecordingEgressIDUnique guards the poller's key. It looks rows
// up by the id LiveKit reports, so two rows sharing one id would have it
// finishing an arbitrary one of them.
func TestConferenceRecordingEgressIDUnique(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, firstIDs := mkConference(t, owner, "Первая")
	secondIDs := owner.expect(t, owner.post("/workspaces/"+s.WS+"/conferences",
		map[string]any{"title": "Вторая"}), http.StatusCreated)["id"].(string)

	egress := "EG_" + uuid.NewString()
	if _, err := mkRecording(t, mustUUID(t, firstIDs), egress, nil); err != nil {
		t.Fatalf("first recording: %v", err)
	}
	if _, err := mkRecording(t, mustUUID(t, secondIDs), egress, nil); !uniqueViolation(err) {
		t.Fatalf("a duplicate egress id was accepted (err=%v), want a unique violation", err)
	}
}

// TestFinishConferenceRecordingIsFinal covers the WHERE status = 'active' guard.
//
// An explicit stop and the poller can reach the same recording at once — the
// stop returns LiveKit's final state, and the tick that is already in flight
// carries the same one. A second write would be harmless for the status but not
// for expires_at: it would push a fresh TTL from *now*, and a recording that
// should vanish in a week would outlive every sweep.
func TestFinishConferenceRecordingIsFinal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := signup(t)
	_, confIDs := mkConference(t, owner, "Летучка")
	by := mustUUID(t, owner.UserID)

	rec, err := mkRecording(t, mustUUID(t, confIDs), "EG_"+uuid.NewString(), &by)
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	done, err := testQueries.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
		ID: rec.ID, Status: "completed", SizeBytes: 5 << 20, DurationSec: 125, TtlDays: 7,
	})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if done.EndedAt == nil {
		t.Fatalf("finish left ended_at unset: %#v", done)
	}
	if done.ExpiresAt == nil {
		t.Fatalf("ttl 7 left expires_at NULL, which means never deleted")
	}
	if got := time.Until(*done.ExpiresAt); got < 6*24*time.Hour || got > 8*24*time.Hour {
		t.Fatalf("expires_at is %v away, want ~7 days", got)
	}

	// The late tick: same row, later TTL, and it must change nothing.
	if _, err := testQueries.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
		ID: rec.ID, Status: "failed", Error: "late tick", TtlDays: 3650,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("finishing a finished recording returned %v, want no rows", err)
	}
	again, err := testQueries.GetConferenceRecording(ctx, rec.ID)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if again.Status != "completed" || again.Error != "" || again.DurationSec != 125 {
		t.Fatalf("the late tick overwrote a finished recording: %#v", again)
	}
	if !again.ExpiresAt.Equal(*done.ExpiresAt) {
		t.Fatalf("expires_at moved from %v to %v — the file would outlive its TTL", done.ExpiresAt, again.ExpiresAt)
	}
}

// TestConferenceRecordingTTLZeroNeverExpires: recording_ttl_days = 0 means keep
// indefinitely, and the sweeper finds rows by expires_at. A zero TTL that stored
// now() + 0 days would delete the recording on the next tick — the opposite of
// what the setting says.
func TestConferenceRecordingTTLZeroNeverExpires(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := signup(t)
	_, confIDs := mkConference(t, owner, "Летучка")

	rec, err := mkRecording(t, mustUUID(t, confIDs), "EG_"+uuid.NewString(), nil)
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	done, err := testQueries.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
		ID: rec.ID, Status: "completed", SizeBytes: 1, DurationSec: 1, TtlDays: 0,
	})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if done.ExpiresAt != nil {
		t.Fatalf("ttl 0 set expires_at = %v, want NULL (keep indefinitely)", done.ExpiresAt)
	}
}

// TestConferenceRecordingViews checks the three read paths the room state, the
// poller and the sweeper use, including what each one must *not* return.
func TestConferenceRecordingViews(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := signup(t)
	_, confIDs := mkConference(t, owner, "Летучка")
	confID := mustUUID(t, confIDs)
	by := mustUUID(t, owner.UserID)

	rec, err := mkRecording(t, confID, "EG_"+uuid.NewString(), &by)
	if err != nil {
		t.Fatalf("recording: %v", err)
	}

	// The red dot in the room: it names who started recording, so the join has
	// to come back with the row.
	live, err := testQueries.ActiveConferenceRecording(ctx, confID)
	if err != nil {
		t.Fatalf("active recording: %v", err)
	}
	if live.ID != rec.ID || live.StartedByName == nil || *live.StartedByName == "" {
		t.Fatalf("the indicator has no name to show: %#v", live)
	}

	// The poller's list carries the conference's TTL so it does not read it row
	// by row a minute later.
	found := false
	active, err := testQueries.ListActiveRecordings(ctx)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	for _, r := range active {
		if r.ID == rec.ID {
			found = true
			if r.RecordingTtlDays != 30 {
				t.Fatalf("ttl carried as %d, want the conference default 30", r.RecordingTtlDays)
			}
		}
	}
	if !found {
		t.Fatalf("a running recording is missing from the poller's list")
	}

	// Authorisation for the download route resolves in one statement.
	conf, err := testQueries.ConferenceForRecording(ctx, rec.ID)
	if err != nil {
		t.Fatalf("conference for recording: %v", err)
	}
	if conf.ID != confID {
		t.Fatalf("resolved conference %s, want %s", conf.ID, confID)
	}

	// The sweeper takes expired rows only. A running recording has no expiry at
	// all, and picking it up would delete a file that is still being written.
	if inExpired(ctx, t, rec.ID) {
		t.Fatalf("the sweeper picked up a running recording")
	}
	if _, err := testQueries.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
		ID: rec.ID, Status: "completed", SizeBytes: 10, DurationSec: 3, TtlDays: 1,
	}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if inExpired(ctx, t, rec.ID) {
		t.Fatalf("a recording that expires tomorrow was swept today")
	}
	if _, err := testPool.Exec(ctx,
		`UPDATE conference_recordings SET expires_at = now() - interval '1 hour' WHERE id = $1`,
		rec.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if !inExpired(ctx, t, rec.ID) {
		t.Fatalf("an expired recording never reaches the sweeper — the file would stay forever")
	}

	// Whole-conference cleanup reads the paths before the cascade removes the
	// rows that hold them.
	paths, err := testQueries.ListConferenceRecordingPaths(ctx, confID)
	if err != nil {
		t.Fatalf("paths: %v", err)
	}
	if len(paths) != 1 || paths[0] != rec.FilePath {
		t.Fatalf("paths = %v, want [%s]", paths, rec.FilePath)
	}

	list, err := testQueries.ListConferenceRecordings(ctx, confID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Status != "completed" {
		t.Fatalf("the card's list is %#v", list)
	}
	if _, err := testQueries.ActiveConferenceRecording(ctx, confID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("the indicator still sees a live recording after it finished (err=%v)", err)
	}
}

// inExpired reports whether the sweeper's query returns this row. The limit is
// generous because the suite runs in parallel and other tests leave recordings
// of their own behind.
func inExpired(ctx context.Context, t *testing.T, id uuid.UUID) bool {
	t.Helper()
	rows, err := testQueries.ListExpiredRecordings(ctx, 1000)
	if err != nil {
		t.Fatalf("list expired: %v", err)
	}
	for _, r := range rows {
		if r.ID == id {
			return true
		}
	}
	return false
}
