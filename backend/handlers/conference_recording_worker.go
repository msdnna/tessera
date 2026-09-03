package handlers

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/livekit"
)

// The background half of conference recording (#2864, subtask #2877).
//
// The handlers next door close a recording the fast way: the host presses stop
// and a detached poll waits the second or two it takes the egress worker to
// write the file. This worker is what happens when nobody presses anything —
// the call simply emptied and LiveKit ended the egress on its own, the worker
// crashed, or the backend was restarted mid-recording. Without it those rows
// stay "active" forever, which is worse than it sounds: the unique partial
// index that makes "one recording at a time" true would then refuse every
// future recording of that conference, and the red dot would never go out.
//
// It also owns the other end of a recording's life — the TTL sweep that finally
// deletes the mp4. That is the largest file this application writes, and
// recording_ttl_days has been a promise with nothing behind it since 0069.
const (
	// recordingSweepEvery matches the interval published in backgroundWorkers,
	// which is what the jobs screen counts down.
	recordingSweepEvery = time.Minute

	// recordingSweepBatch bounds one TTL pass. Deleting is not the slow part —
	// the first pass after somebody lowers a TTL can be, and a worker that
	// holds its advisory lock for minutes stops answering for everything else
	// it owns. The remainder is picked up by the next tick a minute later.
	recordingSweepBatch = 200

	// recordingClaimGrace is how long a row may name no egress before this
	// worker concludes the process that opened it died.
	//
	// The gap is normally two statements wide — microseconds — so anything
	// still empty after this had no writer for two full ticks. The grace is not
	// politeness: reading such a row as abandoned too early would race the
	// handler that is about to fill it in, and we would fail a recording that
	// is starting perfectly well.
	recordingClaimGrace = 2 * time.Minute

	// recordingMaxLifetime is the wall-clock backstop for a row whose egress
	// never reaches a terminal status.
	//
	// EgressInfo.Done reports only on statuses we know, so a status LiveKit
	// invents later reads as "still running" — deliberately, since a poller
	// that keeps asking is safer than one that declares a live recording lost.
	// The cost of that choice is this constant. It sits above the two-hour
	// session_limits ceiling in deploy/egress.yaml with room to spare, so a
	// recording that hits the ceiling is ended by LiveKit and reported
	// properly; only something genuinely stuck gets here.
	recordingMaxLifetime = 3 * time.Hour

	// recordingWorkerOpTimeout bounds one call to LiveKit or one statement.
	recordingWorkerOpTimeout = 30 * time.Second

	errRecordingStuck = "recording did not finish and was cut off"
)

// RunConferenceRecordingWorker drives the recording lifecycle: it finishes rows
// whose egress ended without anyone watching, and deletes recordings whose TTL
// has run out. Idle on an install where nobody records anything.
func (h *API) RunConferenceRecordingWorker(ctx context.Context) {
	ticker := time.NewTicker(recordingSweepEvery)
	defer ticker.Stop()
	h.tick(jobConfRecordings, opRecordingSweep)
	// Catch up at startup: a restart mid-recording is exactly the case where
	// rows are left active with nobody polling them, and waiting a minute to
	// notice would leave the red dot on for that whole minute.
	h.withAdvisoryLock(ctx, "conference_recordings", func() { h.SweepConferenceRecordings(ctx) })
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick(jobConfRecordings, opRecordingSweep)
			h.withAdvisoryLock(ctx, "conference_recordings", func() { h.SweepConferenceRecordings(ctx) })
		}
	}
}

// SweepConferenceRecordings runs one pass of both halves.
//
// Exported because a single pass is the unit everything else wants: the loop
// above, the "run now" button on the jobs screen, and the integration tests,
// which drive one pass against a stub SFU rather than waiting on a ticker.
func (h *API) SweepConferenceRecordings(ctx context.Context) {
	h.finalizeStaleRecordings(ctx)
	h.sweepExpiredRecordings(ctx)
}

// finalizeStaleRecordings closes every row LiveKit no longer considers running.
func (h *API) finalizeStaleRecordings(ctx context.Context) {
	// Nothing to ask without an SFU. This is not just an optimisation: a
	// deployment that turned recording off still has its old rows, and a
	// disabled client would report every one of them as unreachable — marking a
	// pile of finished recordings failed on the way past.
	if h.livekit == nil || !h.livekit.Enabled() {
		return
	}
	rows, err := h.q.ListActiveRecordings(ctx)
	if err != nil {
		soft(ctx, "conference.recording.list-active", err)
		return
	}
	for _, row := range rows {
		select {
		case <-ctx.Done():
			return
		default:
		}
		h.finalizeRecording(ctx, row)
	}
}

// finalizeRecording decides the fate of one in-flight recording row.
func (h *API) finalizeRecording(ctx context.Context, row db.ListActiveRecordingsRow) {
	owner := recordingOwner{
		ConferenceID: row.ConferenceID, WorkspaceID: row.WorkspaceID, TTLDays: row.RecordingTtlDays,
	}
	if row.EgressID == "" {
		h.recoverUnclaimedRecording(ctx, row, owner)
		return
	}

	opCtx, cancel := context.WithTimeout(ctx, recordingWorkerOpTimeout)
	info, err := h.livekit.GetEgress(opCtx, row.EgressID)
	cancel()
	switch {
	case errors.Is(err, livekit.ErrEgressGone):
		// livekit-server forgets finished egresses after a while, so this is
		// the ordinary ending for a recording nobody was polling when it
		// stopped — and it is also what a lost job looks like. The two are
		// indistinguishable from here, which is why the file is checked rather
		// than assumed: an mp4 of a real size on disk means the worker did
		// finish its job before the server forgot about it.
		h.finishForgottenRecording(ctx, row, owner)
	case err != nil:
		// Transient: the SFU is restarting or the network blinked. The row
		// stays active and the next tick asks again.
		soft(ctx, "conference.recording.poll", err)
	case info.Done():
		status := "failed"
		if info.Succeeded() {
			status = "completed"
		}
		h.finishRecordingCtx(ctx, row.ID, owner, status, resultOf(info))
	case time.Since(row.StartedAt) > recordingMaxLifetime:
		// Still "running" hours after the session limit should have ended it.
		// Stop it before closing the row: the row is our only handle on that
		// worker, and dropping it here would leave a headless Chrome writing
		// into the uploads volume with nothing left that knows its id.
		opCtx, cancel := context.WithTimeout(ctx, recordingWorkerOpTimeout)
		if _, serr := h.livekit.StopEgress(opCtx, row.EgressID); serr != nil {
			soft(ctx, "conference.recording.stop-stuck", serr)
		}
		cancel()
		h.finishRecordingCtx(ctx, row.ID, owner, "failed", recordingResult{Err: errRecordingStuck})
	}
}

// recoverUnclaimedRecording handles a row that names no egress: the process
// died between claiming the row and attaching LiveKit's job id.
//
// Adoption first, because the two outcomes are not equally bad. The egress may
// well be running — the start call is what happens between those two statements
// — and failing the row would leave that worker recording into a file no row
// points at, until the session limit stops it hours later. Asking the room
// costs one call and gets the id back.
func (h *API) recoverUnclaimedRecording(ctx context.Context, row db.ListActiveRecordingsRow, owner recordingOwner) {
	if time.Since(row.StartedAt) < recordingClaimGrace {
		return // the handler that opened this row is still mid-flight
	}
	opCtx, cancel := context.WithTimeout(ctx, recordingWorkerOpTimeout)
	list, err := h.livekit.ListEgress(opCtx, livekit.RoomName(row.ConferenceID), false)
	cancel()
	if err != nil {
		soft(ctx, "conference.recording.recover", err)
		return // try again next tick rather than fail a recording that may be fine
	}
	for _, info := range list {
		// Only an unfinished egress can belong to this row. A finished one on
		// the same room is a previous recording of the same conference, and
		// adopting it would hand this row somebody else's file.
		if info.Done() {
			continue
		}
		// Only the id is repaired. The red dot is not lit from here: the room
		// seeds it from this same row when the first socket arrives, and that
		// path already refuses a reading that a stop has since overtaken —
		// lighting it a second way would be a second chance to get that wrong.
		if _, cerr := h.q.SetConferenceRecordingEgress(ctx, db.SetConferenceRecordingEgressParams{
			ID: row.ID, EgressID: info.EgressID,
		}); cerr != nil {
			soft(ctx, "conference.recording.adopt", cerr)
			return
		}
		return
	}
	h.finishRecordingCtx(ctx, row.ID, owner, "failed", recordingResult{Err: errRecordingNeverStarted})
}

// finishForgottenRecording closes a row whose egress LiveKit no longer knows
// about, reading the file to decide whether that was an ending or a loss.
func (h *API) finishForgottenRecording(ctx context.Context, row db.ListActiveRecordingsRow, owner recordingOwner) {
	st, err := os.Stat(row.FilePath)
	if err != nil || st.Size() == 0 {
		h.finishRecordingCtx(ctx, row.ID, owner, "failed", recordingResult{Err: errRecordingLost})
		return
	}
	// A file with bytes in it is a recording somebody can watch, whatever the
	// server has since forgotten. Duration is left at zero rather than guessed
	// from timestamps: started_at is ours and the file's own length is not, and
	// a made-up number in that column would be indistinguishable from a
	// measured one.
	h.finishRecordingCtx(ctx, row.ID, owner, "completed", recordingResult{Size: st.Size()})
}

// finishRecordingCtx is finishRecording with its own timeout, so one wedged
// statement cannot hold up the rest of the pass.
func (h *API) finishRecordingCtx(ctx context.Context, recID uuid.UUID, owner recordingOwner, status string, res recordingResult) {
	opCtx, cancel := context.WithTimeout(ctx, recordingWorkerOpTimeout)
	defer cancel()
	h.finishRecording(opCtx, recID, owner, status, res)
}

// sweepExpiredRecordings deletes recordings whose TTL has run out — file first,
// then row.
//
// That order is the whole point. A row deleted before its file leaves an mp4
// nothing references and no later pass can find, and these are the biggest
// files on the disk; a file deleted before its row leaves a row whose download
// 404s until the next tick removes it, which is a page refresh, not a leak.
func (h *API) sweepExpiredRecordings(ctx context.Context) {
	rows, err := h.q.ListExpiredRecordings(ctx, recordingSweepBatch)
	if err != nil {
		soft(ctx, "conference.recording.list-expired", err)
		return
	}
	for _, row := range rows {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// A file that is already gone is not a reason to keep the row: an
		// operator who cleared the volume by hand would otherwise wedge the
		// sweeper on the same batch forever, and the rows would outlive the
		// recordings they describe.
		if err := os.Remove(row.FilePath); err != nil && !os.IsNotExist(err) {
			soft(ctx, "conference.recording.unlink", err)
			continue
		}
		opCtx, cancel := context.WithTimeout(ctx, recordingWorkerOpTimeout)
		derr := h.q.DeleteConferenceRecording(opCtx, row.ID)
		cancel()
		if derr != nil {
			soft(ctx, "conference.recording.sweep", derr)
			continue
		}
		h.broadcast(row.WorkspaceID, "conference.recording.deleted", map[string]any{
			"id": row.ID, "conference_id": row.ConferenceID, "expired": true,
		})
	}
}
