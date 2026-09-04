// Conference recording over the real HTTP surface (#2864, subtask #2877).
//
// internal/confroom proves the red dot behaves, and the store tests prove the
// schema refuses a second live recording. What only this level can show is the
// part that decides whether the feature is safe: who may start one, who may
// watch it back, and whether deleting things actually removes the mp4 from disk
// — the largest file this application ever writes.
//
// The harness runs without LIVEKIT_*, so the SFU client reports itself disabled
// and start/stop answer 503. That is not a gap papered over: it is the same path
// a self-hosted install without conferences takes, and the ordering it exposes
// (permission first, capability second) is worth pinning on its own. The rows
// the download and delete tests need are planted through the same queries the
// handler uses, so everything after the egress call is exercised for real.
package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"tessera/internal/db"
)

// liveConference schedules a conference and joins it, because recording is
// refused on a room nobody is in.
func liveConference(t *testing.T, c *client, title string) (s stack, confID string) {
	t.Helper()
	s, confID = mkConference(t, c, title)
	c.expect(t, c.post("/conferences/"+confID+"/join", nil), http.StatusOK)
	return s, confID
}

// plantRecording writes a recording row with a real file behind it, standing in
// for the egress worker the harness has no way to run. status "active" leaves
// the row in flight; anything else closes it through the same statement the
// handler uses.
func plantRecording(t *testing.T, confID, status string) (recID, path string) {
	t.Helper()
	ctx := context.Background()
	cid := uuid.MustParse(confID)
	dir := filepath.Join(testUploadDir, "rec", confID)
	if err := os.MkdirAll(dir, 0o775); err != nil {
		t.Fatalf("recording dir: %v", err)
	}
	path = filepath.Join(dir, uuid.NewString()+".mp4")
	if err := os.WriteFile(path, []byte("fake-mp4-bytes"), 0o644); err != nil {
		t.Fatalf("recording file: %v", err)
	}
	rec, err := testQueries.CreateConferenceRecording(ctx, db.CreateConferenceRecordingParams{
		ConferenceID: cid, FilePath: path, FileName: "rec-2026-09-03-2300.mp4",
	})
	if err != nil {
		t.Fatalf("plant recording: %v", err)
	}
	if status != "active" {
		if _, err := testQueries.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
			ID: rec.ID, Status: status, SizeBytes: 14, DurationSec: 42, TtlDays: 30,
		}); err != nil {
			t.Fatalf("finish planted recording: %v", err)
		}
	}
	return rec.ID.String(), path
}

// TestRecordingStartChecksPermissionBeforeCapability: an install with no SFU
// answers 503, but a plain member must never learn that much. Getting this order
// wrong turns "you may not record this meeting" into "recording is off on this
// server", which is a different — and wrong — answer.
func TestRecordingStartChecksPermissionBeforeCapability(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := liveConference(t, owner, "Летучка без записи")
	mate := addMember(t, owner, s.WS)

	if r := mate.post("/conferences/"+confID+"/recording/start", nil); r.Status != http.StatusForbidden {
		t.Fatalf("member start: status %d, want 403\n%s", r.Status, r.Body)
	}
	if r := owner.post("/conferences/"+confID+"/recording/start", nil); r.Status != http.StatusServiceUnavailable {
		t.Fatalf("host start without LIVEKIT_*: status %d, want 503\n%s", r.Status, r.Body)
	}
	// Nothing was claimed: a 503 that left an active row behind would block every
	// future recording of this conference on the one-active index.
	if got := owner.get("/conferences/" + confID + "/recordings").listBody(t); len(got) != 0 {
		t.Fatalf("recordings after a refused start = %d, want 0", len(got))
	}
}

// TestRecordingStopWithoutOneIs409: the button is idempotent-looking to a user,
// but "nothing is running" is a different answer from "stopped", and the client
// draws them differently.
func TestRecordingStopWithoutOneIs409(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка без записи 2")

	if r := owner.post("/conferences/"+confID+"/recording/stop", nil); r.Status != http.StatusConflict {
		t.Fatalf("stop with nothing running: status %d, want 409\n%s", r.Status, r.Body)
	}
}

// TestRecordingListIsForMembersOnly: a recording is the meeting itself. Everyone
// who could have attended may watch it back, and nobody else can even learn it
// exists.
func TestRecordingListIsForMembersOnly(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := liveConference(t, owner, "Летучка со списком")
	mate := addMember(t, owner, s.WS)
	outsider := signup(t)

	plantRecording(t, confID, "completed")

	if got := owner.get("/conferences/" + confID + "/recordings").listBody(t); len(got) != 1 {
		t.Fatalf("host sees %d recordings, want 1", len(got))
	}
	if got := mate.get("/conferences/" + confID + "/recordings").listBody(t); len(got) != 1 {
		t.Fatalf("member sees %d recordings, want 1", len(got))
	}
	if r := outsider.get("/conferences/" + confID + "/recordings"); r.Status != http.StatusForbidden {
		t.Fatalf("outsider list: status %d, want 403\n%s", r.Status, r.Body)
	}
}

// TestRecordingDownloadNeedsAFinishedRecording: an in-flight row has no file
// yet, and answering 404 would read as "your recording is gone" rather than
// "it is still being written".
func TestRecordingDownloadNeedsAFinishedRecording(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка на скачивание")
	recID, _ := plantRecording(t, confID, "active")

	if r := owner.get("/conference-recordings/" + recID + "/download"); r.Status != http.StatusConflict {
		t.Fatalf("download of a running recording: status %d, want 409\n%s", r.Status, r.Body)
	}
}

// TestRecordingDownloadServesTheFileToMembersOnly walks the whole download path:
// the bytes come back, they are marked as a download rather than something to
// render on our own origin, and an outsider gets nothing.
func TestRecordingDownloadServesTheFileToMembersOnly(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка со скачиванием")
	recID, _ := plantRecording(t, confID, "completed")
	outsider := signup(t)

	r := owner.get("/conference-recordings/" + recID + "/download")
	if r.Status != http.StatusOK {
		t.Fatalf("download: status %d, want 200\n%s", r.Status, r.Body)
	}
	if string(r.Body) != "fake-mp4-bytes" {
		t.Fatalf("download body = %q", r.Body)
	}
	if got := r.Header.Get("Content-Disposition"); got != `attachment; filename="rec-2026-09-03-2300.mp4"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	// Attacker-supplied bytes behind nothing but a bearer token must not be
	// sniffable into something the browser will execute on our origin.
	if got := r.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if r := outsider.get("/conference-recordings/" + recID + "/download"); r.Status != http.StatusForbidden {
		t.Fatalf("outsider download: status %d, want 403\n%s", r.Status, r.Body)
	}
}

// TestRecordingDeleteIsModerationAndUnlinksTheFile: the row and the mp4 have to
// go together. A delete that dropped only the row would leave the biggest file
// this app writes on disk with nothing left pointing at it — invisible to the
// TTL sweep and to every screen.
func TestRecordingDeleteIsModerationAndUnlinksTheFile(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	s, confID := liveConference(t, owner, "Летучка на удаление")
	mate := addMember(t, owner, s.WS)
	recID, path := plantRecording(t, confID, "completed")

	if r := mate.del("/conference-recordings/" + recID); r.Status != http.StatusForbidden {
		t.Fatalf("member delete: status %d, want 403\n%s", r.Status, r.Body)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a refused delete removed the file: %v", err)
	}
	if r := owner.del("/conference-recordings/" + recID); r.Status != http.StatusNoContent {
		t.Fatalf("host delete: status %d, want 204\n%s", r.Status, r.Body)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("recording file survived the delete: %v", err)
	}
	if got := owner.get("/conferences/" + confID + "/recordings").listBody(t); len(got) != 0 {
		t.Fatalf("recordings after delete = %d, want 0", len(got))
	}
}

// TestRunningRecordingCannotBeDeleted: dropping the row of a live recording
// leaves the egress worker writing to a path nothing remembers.
func TestRunningRecordingCannotBeDeleted(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка с живой записью")
	recID, path := plantRecording(t, confID, "active")

	if r := owner.del("/conference-recordings/" + recID); r.Status != http.StatusConflict {
		t.Fatalf("delete of a running recording: status %d, want 409\n%s", r.Status, r.Body)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a refused delete removed the file: %v", err)
	}
}

// TestDeletingAConferenceRemovesItsRecordings: the rows leave by cascade, which
// is exactly why the files have to be unlinked by hand — after the delete
// nothing remembers where they were.
func TestDeletingAConferenceRemovesItsRecordings(t *testing.T) {
	t.Parallel()
	owner := signup(t)
	_, confID := liveConference(t, owner, "Летучка под снос")
	_, first := plantRecording(t, confID, "completed")
	_, second := plantRecording(t, confID, "failed")

	if r := owner.del("/conferences/" + confID); r.Status != http.StatusNoContent {
		t.Fatalf("delete conference: status %d, want 204\n%s", r.Status, r.Body)
	}
	for _, p := range []string{first, second} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("recording file %s survived the conference: %v", filepath.Base(p), err)
		}
	}
}
