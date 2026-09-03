package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"tessera/internal/confroom"
	"tessera/internal/db"
	"tessera/internal/livekit"
	"tessera/internal/observability"
	"tessera/middleware"
)

// Server-side conference recording (#2864, subtask #2877): the HTTP half.
//
// Nothing here records anything. LiveKit's egress worker joins the call with a
// headless Chrome and writes the mp4 straight into the uploads volume both
// containers share; what this file owns is the row that says a recording exists,
// the permission to start one, and the indicator that tells the room it is
// happening.
//
// Three decisions shape the handlers below.
//
//  1. The database row is claimed *before* the egress is started. The unique
//     partial index on (conference_id) WHERE status='active' is then a real
//     mutex: two hosts pressing record in the same second means the loser is
//     refused before a second Chrome joins the call. The reverse order would
//     leave the loser holding a worker it has to stop, and a stop that fails
//     writes a file nothing points at, forever.
//
//  2. The red dot is everybody's, not the host's. It travels in the room
//     snapshot (confroom.StateMsg), so a client that reconnects re-reads it and
//     a client that joins mid-recording sees it at once. Recording people
//     without telling them is the one thing this feature must not do.
//
//  3. A recording is finished by polling, not by a webhook. The file exists only
//     when the egress ends, and a webhook would mean a new public endpoint with
//     its own signature check — a permanent attack surface for one event a day.
//     An explicit stop starts a short poll here; everything else (a call that
//     simply emptied, a crashed worker, a restarted backend) is the background
//     worker's job.

const (
	// recordingSubdir keeps recordings out of conf/, where the chat's
	// attachments live. Not tidiness: the two directories have different
	// writers. Chat files are written by the backend (uid 65532 in the shipped
	// image), recordings by the egress container (uid 1001), and the only way to
	// let both write is a shared group and a group-writable directory. Applying
	// that to conf/ would have meant changing the ownership of a directory the
	// backend already creates subdirectories in — and getting it wrong there
	// breaks chat uploads, which have nothing to do with recording.
	recordingSubdir = "rec"

	// recordingDirMode is group-writable on purpose: the backend creates the
	// per-conference directory, the egress worker creates the file inside it.
	// deploy/docker-compose.yml sets the setgid bit on the parent so the group
	// is inherited; without the group-write bit here the recording would fail
	// late and quietly — Chrome joins, the call is recorded, and only the final
	// write fails.
	recordingDirMode = 0o775

	// recordingPollEvery / recordingPollFor bound the finalisation poll that
	// follows an explicit stop. The file is written when the worker winds down,
	// a second or two after LiveKit acknowledges the stop, so waiting for the
	// once-a-minute background sweep would leave the dot on and the recording
	// missing from the list for far longer than the user's patience.
	recordingPollEvery = 2 * time.Second
	recordingPollFor   = 45 * time.Second

	// recordingOpTimeout bounds one detached finalisation round.
	recordingOpTimeout = 30 * time.Second
)

// recordingView is one recording as clients see it.
//
// file_path is deliberately absent: it is a path on the server's disk, of no use
// to a client and of some use to anyone probing the install. The download route
// resolves it from the id instead.
type recordingView struct {
	ID            uuid.UUID  `json:"id"`
	ConferenceID  uuid.UUID  `json:"conference_id"`
	Status        string     `json:"status"`
	Error         string     `json:"error"`
	FileName      string     `json:"file_name"`
	SizeBytes     int64      `json:"size_bytes"`
	DurationSec   int32      `json:"duration_sec"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	StartedBy     *uuid.UUID `json:"started_by"`
	StartedByName *string    `json:"started_by_name"`
}

func viewRecording(r db.ConferenceRecording) recordingView {
	return recordingView{
		ID: r.ID, ConferenceID: r.ConferenceID, Status: r.Status, Error: r.Error,
		FileName: r.FileName, SizeBytes: r.SizeBytes, DurationSec: r.DurationSec,
		StartedAt: r.StartedAt, EndedAt: r.EndedAt, ExpiresAt: r.ExpiresAt,
		StartedBy: r.StartedBy,
	}
}

func viewRecordingRow(r db.ListConferenceRecordingsRow) recordingView {
	return recordingView{
		ID: r.ID, ConferenceID: r.ConferenceID, Status: r.Status, Error: r.Error,
		FileName: r.FileName, SizeBytes: r.SizeBytes, DurationSec: r.DurationSec,
		StartedAt: r.StartedAt, EndedAt: r.EndedAt, ExpiresAt: r.ExpiresAt,
		StartedBy: r.StartedBy, StartedByName: r.StartedByName,
	}
}

// StartConferenceRecording asks LiveKit to record the call and opens the row
// that tracks it.
func (h *API) StartConferenceRecording(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	if !h.requireConferenceManager(c, conf) {
		return
	}
	if h.livekit == nil || !h.livekit.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "recording is not configured on this server"})
		return
	}
	// Recording a room nobody is in produces a file of an empty grid and a worker
	// that has to be timed out by the session limit. The status is also what the
	// caller's own screen shows, so refusing here matches what they can see.
	if conf.Status != "live" {
		c.JSON(http.StatusConflict, gin.H{"error": "conference is not running"})
		return
	}

	// The name on disk is a uuid, like every other upload: it is written by
	// another container into a shared directory, and a predictable name is one
	// more thing that could collide. FileName is what the download is called.
	diskName := uuid.NewString() + ".mp4"
	fileName := "rec-" + time.Now().Format("2006-01-02-1504") + ".mp4"
	dir, err := h.recordingDir(conf.ID)
	if err != nil {
		fail(c, err)
		return
	}
	uid := middleware.CurrentUser(c)
	rec, err := h.q.CreateConferenceRecording(c, db.CreateConferenceRecordingParams{
		ConferenceID: conf.ID,
		FilePath:     filepath.Join(dir, diskName),
		FileName:     fileName,
		StartedBy:    &uid,
	})
	if isUniqueViolation(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "recording is already running"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}

	// path.Join, not filepath.Join: this one is a path inside the egress
	// container, which is Linux regardless of what the backend runs on.
	target := path.Join(h.egressUploadDir(), recordingSubdir, conf.ID.String(), diskName)
	info, err := h.livekit.StartRoomCompositeEgress(c, livekit.RoomName(conf.ID), livekit.EgressOptions{
		Filepath: target,
	})
	if err != nil {
		h.releaseRecordingClaim(c, rec.ID)
		c.JSON(http.StatusBadGateway, gin.H{"error": "recording could not be started: " + err.Error()})
		return
	}
	rec, err = h.q.SetConferenceRecordingEgress(c, db.SetConferenceRecordingEgressParams{
		ID: rec.ID, EgressID: info.EgressID,
	})
	if err != nil {
		// The worker is already running and we just lost our only handle on it.
		// Stop it now rather than leave it writing into the volume until the
		// two-hour session limit fires.
		if _, serr := h.livekit.StopEgress(c, info.EgressID); serr != nil {
			soft(c, "conference.recording.orphan-stop", serr)
		}
		h.releaseRecordingClaim(c, rec.ID)
		fail(c, err)
		return
	}

	view := viewRecording(rec)
	if name := h.userName(c, uid); name != "" {
		view.StartedByName = &name
		h.setConfRecording(conf.ID, &confroom.RecordingView{
			ID: rec.ID.String(), StartedAt: rec.StartedAt, StartedBy: name,
		})
	} else {
		h.setConfRecording(conf.ID, &confroom.RecordingView{
			ID: rec.ID.String(), StartedAt: rec.StartedAt,
		})
	}
	h.broadcastAs(c, conf.WorkspaceID, "conference.recording.started", view)
	c.JSON(http.StatusCreated, view)
}

// releaseRecordingClaim drops a row whose egress never got going, so the next
// press of the button is not refused by our own bookkeeping.
//
// Deleted rather than marked failed: the caller is being told about the failure
// in the response they are still waiting for, and a row for a recording that
// never existed would only clutter the list with something they cannot play.
func (h *API) releaseRecordingClaim(c *gin.Context, id uuid.UUID) {
	if err := h.q.DeleteConferenceRecording(c, id); err != nil {
		soft(c, "conference.recording.rollback", err)
	}
}

// StopConferenceRecording ends a running recording.
//
// It does not close the row: the mp4 is written while the worker winds down, so
// its size and duration are not knowable yet. The row is finished by the poll
// this kicks off — or, if that outlives the request, by the background worker.
func (h *API) StopConferenceRecording(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	if !h.requireConferenceManager(c, conf) {
		return
	}
	active, err := h.q.ActiveConferenceRecording(c, conf.ID)
	if errIsNoRows(err) {
		c.JSON(http.StatusConflict, gin.H{"error": "recording is not running"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if h.livekit == nil || !h.livekit.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "recording is not configured on this server"})
		return
	}
	// An empty egress id means the process died between claiming the row and
	// attaching the job. There is nothing to stop, and leaving the row active
	// would block every future recording of this conference on the unique index.
	if active.EgressID == "" {
		h.finishRecording(c, active.ID, conf, "failed", livekit.EgressInfo{Error: "recording was never started"})
		c.JSON(http.StatusOK, gin.H{"status": "failed"})
		return
	}

	switch _, err := h.livekit.StopEgress(c, active.EgressID); {
	case err == nil:
	case errors.Is(err, livekit.ErrEgressGone):
		// LiveKit has forgotten this job, so nothing will ever report on it
		// again. Close the row now instead of polling something that is gone.
		h.finishRecording(c, active.ID, conf, "failed", livekit.EgressInfo{Error: "recording was lost by the media server"})
		c.JSON(http.StatusOK, gin.H{"status": "failed"})
		return
	default:
		var lkErr *livekit.Error
		// "Already stopped" is the ordinary race, not a failure: the host presses
		// stop as the call empties and the worker is winding down on its own. The
		// recording they wanted ended is ending.
		if !errors.As(err, &lkErr) || !lkErr.AlreadyStopped() {
			c.JSON(http.StatusBadGateway, gin.H{"error": "recording could not be stopped: " + err.Error()})
			return
		}
	}
	h.pollRecordingDone(active.ID, active.EgressID, conf)
	c.JSON(http.StatusAccepted, gin.H{"status": "stopping"})
}

// pollRecordingDone waits for a stopping egress to produce its file and then
// closes the row.
//
// Detached from the request on purpose: the host who pressed stop gets their
// answer immediately, and nothing about finishing the row needs their
// connection. Bounded, because an egress that never reports is exactly the case
// the background worker exists for — this is the fast path, not the guarantee.
func (h *API) pollRecordingDone(recID uuid.UUID, egressID string, conf db.Conference) {
	go func() {
		defer observability.Recover("conf-recording.finalize")
		deadline := time.Now().Add(recordingPollFor)
		for time.Now().Before(deadline) {
			time.Sleep(recordingPollEvery)
			ctx, cancel := context.WithTimeout(context.Background(), recordingOpTimeout)
			info, err := h.livekit.GetEgress(ctx, egressID)
			cancel()
			switch {
			case errors.Is(err, livekit.ErrEgressGone):
				ctx, cancel := context.WithTimeout(context.Background(), recordingOpTimeout)
				h.finishRecording(ctx, recID, conf, "failed", livekit.EgressInfo{Error: "recording was lost by the media server"})
				cancel()
				return
			case err != nil:
				continue // transient; the deadline above is what ends this loop
			case !info.Done():
				continue
			}
			status := "failed"
			if info.Succeeded() {
				status = "completed"
			}
			ctx, cancel = context.WithTimeout(context.Background(), recordingOpTimeout)
			h.finishRecording(ctx, recID, conf, status, info)
			cancel()
			return
		}
	}()
}

// finishRecording closes a recording row and takes the red dot off the room.
//
// Exported behaviour worth stating: it is safe to call twice and safe to lose a
// race with the background worker. FinishConferenceRecording only touches rows
// that are still active, so the loser updates nothing — which is the point, as
// the winner has already set ended_at and the expiry the sweeper reads.
func (h *API) finishRecording(ctx context.Context, recID uuid.UUID, conf db.Conference, status string, info livekit.EgressInfo) {
	rec, err := h.q.FinishConferenceRecording(ctx, db.FinishConferenceRecordingParams{
		ID:          recID,
		Status:      status,
		SizeBytes:   int64(info.File().Size),
		DurationSec: int32(info.File().Length() / time.Second),
		Error:       info.Error,
		TtlDays:     conf.RecordingTtlDays,
	})
	if errIsNoRows(err) {
		// Somebody else closed it first. The dot is theirs to clear too, but
		// clearing it again is idempotent and costs one comparison.
		h.clearConfRecording(conf.ID, recID.String())
		return
	}
	if err != nil {
		soft(ctx, "conference.recording.finish", err)
		return
	}
	// A failed recording's partial file is the one nobody will ever ask to keep,
	// and with recording_ttl_days = 0 ("keep indefinitely") the sweeper would
	// never come for it — expires_at stays NULL. Unlink it here instead.
	if status == "failed" {
		_ = os.Remove(rec.FilePath)
	}
	h.clearConfRecording(conf.ID, recID.String())
	h.broadcast(conf.WorkspaceID, "conference.recording.finished", viewRecording(rec))
}

// ListConferenceRecordings returns a conference's recordings, newest first.
// Membership in the workspace is enough to see them: they are the meeting, and
// everyone who could attend it can watch it back.
func (h *API) ListConferenceRecordings(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	rows, err := h.q.ListConferenceRecordings(c, conf.ID)
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]recordingView, 0, len(rows))
	for _, r := range rows {
		out = append(out, viewRecordingRow(r))
	}
	c.JSON(http.StatusOK, out)
}

// DownloadConferenceRecording streams a finished recording to a member of the
// conference's workspace.
//
// Served as a download with nosniff, like the chat attachments next door: a
// client that wants to play it inline fetches these bytes with its bearer
// credential and plays them from a blob, so nothing here needs to be renderable
// on the app's own origin.
func (h *API) DownloadConferenceRecording(c *gin.Context) {
	rec, _, ok := h.recordingScope(c)
	if !ok {
		return
	}
	// An active row has no file yet — the worker is still writing it. A 404 here
	// would read as "your recording is gone".
	if rec.Status != "completed" {
		c.JSON(http.StatusConflict, gin.H{"error": "recording is not finished"})
		return
	}
	if _, err := os.Stat(rec.FilePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "файл недоступен"})
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", rec.FileName))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", "video/mp4")
	c.File(rec.FilePath)
}

// DeleteConferenceRecording removes a recording and its file. Moderators only —
// the same boundary that decides who may start one.
func (h *API) DeleteConferenceRecording(c *gin.Context) {
	rec, conf, ok := h.recordingScope(c)
	if !ok {
		return
	}
	if !h.requireConferenceManager(c, conf) {
		return
	}
	// Deleting the row of a running recording would leave the worker writing to a
	// path nothing remembers — an orphan file that no sweep will ever find.
	if rec.Status == "active" {
		c.JSON(http.StatusConflict, gin.H{"error": "stop the recording first"})
		return
	}
	if err := h.q.DeleteConferenceRecording(c, rec.ID); err != nil {
		fail(c, err)
		return
	}
	_ = os.Remove(rec.FilePath) // best-effort; the row is already gone
	h.broadcastAs(c, conf.WorkspaceID, "conference.recording.deleted", gin.H{
		"id": rec.ID, "conference_id": conf.ID,
	})
	c.Status(http.StatusNoContent)
}

// recordingScope resolves a recording from the :id path param together with its
// conference, and authorizes the caller through the conference's workspace.
func (h *API) recordingScope(c *gin.Context) (db.ConferenceRecording, db.Conference, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.ConferenceRecording{}, db.Conference{}, false
	}
	rec, err := h.q.GetConferenceRecording(c, id)
	if notFound(c, err) {
		return db.ConferenceRecording{}, db.Conference{}, false
	}
	if err != nil {
		fail(c, err)
		return db.ConferenceRecording{}, db.Conference{}, false
	}
	conf, err := h.q.ConferenceForRecording(c, id)
	if notFound(c, err) {
		return db.ConferenceRecording{}, db.Conference{}, false
	}
	if err != nil {
		fail(c, err)
		return db.ConferenceRecording{}, db.Conference{}, false
	}
	if !h.requireMember(c, conf.WorkspaceID) {
		return db.ConferenceRecording{}, db.Conference{}, false
	}
	return rec, conf, true
}

// recordingDir creates the directory this conference's recordings live in and
// returns its path as the backend sees it.
//
// The explicit Chmod is not redundant with MkdirAll's mode: MkdirAll applies the
// process umask, which is 022 in the shipped image and would strip exactly the
// group-write bit the egress worker needs. Without it the recording fails at its
// very last step, after the call has already been recorded.
func (h *API) recordingDir(confID uuid.UUID) (string, error) {
	dir := filepath.Join(h.uploadDir, recordingSubdir, confID.String())
	if err := os.MkdirAll(dir, recordingDirMode); err != nil {
		return "", err
	}
	if err := os.Chmod(dir, recordingDirMode); err != nil {
		return "", err
	}
	return dir, nil
}

// egressUploadDir is the uploads volume as the recorder sees it. Falling back to
// our own path is right for the shipped compose, where both containers mount it
// at /data/uploads.
func (h *API) egressUploadDir() string {
	if h.egressDir != "" {
		return h.egressDir
	}
	return h.uploadDir
}

// isUniqueViolation reports Postgres's 23505. Used where a unique index is doing
// real work rather than guarding against a bug — here, the one-active-recording
// index that turns a race between two hosts into a refusal for one of them.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
