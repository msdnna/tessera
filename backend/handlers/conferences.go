package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/middleware"
)

// Conferences (#2864, subtask #2868): the resource layer for scheduled calls.
//
// Media never passes through here — it goes to the LiveKit SFU, and the access
// token for it is issued by the media subtask. What this file owns is the
// bookkeeping: which conferences exist, who is invited, who is actually in the
// room, and when the call started and ended.
//
// Two rules shape every handler below:
//
//  1. Permission is workspace membership, checked through requireMember on the
//     conference's own workspace. There is no separate conference ACL; role
//     inside a call ('host'/'member') decides only who may kick and force-mute.
//  2. Nothing here trusts the client about who it is. The acting user always
//     comes from middleware.CurrentUser — a join/leave for someone else is not
//     expressible in the API surface.

// conferenceScope resolves a conference from the :id path param and authorizes
// the caller through its workspace. It writes the response and returns false on
// a bad id, a missing conference, or a non-member.
func (h *API) conferenceScope(c *gin.Context) (db.Conference, bool) {
	id, ok := parseID(c, "id")
	if !ok {
		return db.Conference{}, false
	}
	conf, err := h.q.GetConference(c, id)
	if notFound(c, err) {
		return db.Conference{}, false
	}
	if err != nil {
		fail(c, err)
		return db.Conference{}, false
	}
	if !h.requireMember(c, conf.WorkspaceID) {
		return db.Conference{}, false
	}
	return conf, true
}

// canManageConference reports whether the caller may edit or delete a
// conference: its creator, a host inside it, or a workspace manager. A plain
// member may join and talk, but must not be able to cancel someone else's
// meeting.
func (h *API) canManageConference(c *gin.Context, conf db.Conference) bool {
	uid := middleware.CurrentUser(c)
	if conf.CreatedBy != nil && *conf.CreatedBy == uid {
		return true
	}
	switch h.memberRole(c, conf.WorkspaceID) {
	case "owner", "admin":
		return true
	}
	p, err := h.q.GetConferenceParticipant(c, db.GetConferenceParticipantParams{
		ConferenceID: conf.ID, UserID: uid,
	})
	return err == nil && p.Role == "host"
}

// CreateConference schedules a conference in a workspace. The creator is
// enrolled as its host in the same transaction: a conference whose only host
// row failed to insert would be a meeting nobody can moderate, and the recovery
// for that is a database edit.
func (h *API) CreateConference(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var req struct {
		Title            string     `json:"title" binding:"required"`
		Description      string     `json:"description"`
		ScheduledAt      *time.Time `json:"scheduled_at"`
		TaskID           *uuid.UUID `json:"task_id"`
		RecordingTTLDays *int32     `json:"recording_ttl_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// A conference may be pinned to a task, but only to one in the same
	// workspace: the link is what later files the protocol back into the task,
	// and a cross-workspace link would put it in front of a team that was never
	// in the call.
	if req.TaskID != nil && !h.taskInWorkspace(c, *req.TaskID, wsID) {
		return
	}
	ttl := int32(30)
	if req.RecordingTTLDays != nil {
		if *req.RecordingTTLDays < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "recording_ttl_days must not be negative"})
			return
		}
		ttl = *req.RecordingTTLDays
	}
	uid := middleware.CurrentUser(c)
	var conf db.Conference
	err := h.inTx(c, func(q *db.Queries) error {
		var err error
		conf, err = q.CreateConference(c, db.CreateConferenceParams{
			WorkspaceID:      wsID,
			TaskID:           req.TaskID,
			CreatedBy:        &uid,
			Title:            req.Title,
			Description:      req.Description,
			ScheduledAt:      req.ScheduledAt,
			RecordingTtlDays: ttl,
		})
		if err != nil {
			return err
		}
		_, err = q.InviteConferenceParticipant(c, db.InviteConferenceParticipantParams{
			ConferenceID: conf.ID, UserID: uid, Role: "host",
		})
		return err
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, wsID, "conference.created", conf)
	c.JSON(http.StatusCreated, conf)
}

// taskInWorkspace checks that a task belongs to the given workspace, writing
// the response and returning false when it does not (or does not exist).
func (h *API) taskInWorkspace(c *gin.Context, taskID, wsID uuid.UUID) bool {
	owner, err := h.q.WorkspaceIDForTask(c, taskID)
	if notFound(c, err) {
		return false
	}
	if err != nil {
		fail(c, err)
		return false
	}
	if owner != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task belongs to another workspace"})
		return false
	}
	return true
}

// ListConferences returns the workspace's conferences, optionally filtered by
// status (scheduled/live/ended).
func (h *API) ListConferences(c *gin.Context) {
	wsID, ok := parseID(c, "id")
	if !ok || !h.requireMember(c, wsID) {
		return
	}
	var status *string
	if raw := c.Query("status"); raw != "" {
		switch raw {
		case "scheduled", "live", "ended":
			status = &raw
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}
	rows, err := h.q.ListConferences(c, db.ListConferencesParams{WorkspaceID: wsID, Status: status})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// GetConference returns one conference together with its participants — the
// room screen needs both to render, and asking for them separately would show
// an empty participants panel for one paint.
func (h *API) GetConference(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	parts, err := h.q.ListConferenceParticipants(c, conf.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"conference": conf, "participants": orEmpty(parts)})
}

// ListTaskConferences returns the conferences held about a task, so the task
// page can link to the meeting where it was discussed.
func (h *API) ListTaskConferences(c *gin.Context) {
	taskID, ok := parseID(c, "id")
	if !ok {
		return
	}
	wsID, err := h.q.WorkspaceIDForTask(c, taskID)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	if !h.requireMember(c, wsID) {
		return
	}
	rows, err := h.q.ListConferencesByTask(c, &taskID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}

// UpdateConference edits a conference's plan. Status is deliberately not
// editable here — start and end have their own endpoints.
func (h *API) UpdateConference(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	if !h.requireConferenceManager(c, conf) {
		return
	}
	req := struct {
		Title            string     `json:"title" binding:"required"`
		Description      string     `json:"description"`
		ScheduledAt      *time.Time `json:"scheduled_at"`
		TaskID           *uuid.UUID `json:"task_id"`
		RecordingTTLDays *int32     `json:"recording_ttl_days"`
	}{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.TaskID != nil && !h.taskInWorkspace(c, *req.TaskID, conf.WorkspaceID) {
		return
	}
	ttl := conf.RecordingTtlDays
	if req.RecordingTTLDays != nil {
		if *req.RecordingTTLDays < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "recording_ttl_days must not be negative"})
			return
		}
		ttl = *req.RecordingTTLDays
	}
	updated, err := h.q.UpdateConference(c, db.UpdateConferenceParams{
		ID:               conf.ID,
		Title:            req.Title,
		Description:      req.Description,
		ScheduledAt:      req.ScheduledAt,
		TaskID:           req.TaskID,
		RecordingTtlDays: ttl,
	})
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcastAs(c, conf.WorkspaceID, "conference.updated", updated)
	c.JSON(http.StatusOK, updated)
}

// requireConferenceManager writes a 403 unless the caller may moderate the
// conference.
func (h *API) requireConferenceManager(c *gin.Context, conf db.Conference) bool {
	if h.canManageConference(c, conf) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "requires the conference host or a workspace admin"})
	return false
}

// DeleteConference removes a conference along with its participants, chat and
// recording rows (ON DELETE CASCADE).
func (h *API) DeleteConference(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	if !h.requireConferenceManager(c, conf) {
		return
	}
	if err := h.q.DeleteConference(c, conf.ID); err != nil {
		fail(c, err)
		return
	}
	h.dropConfRoom(conf.ID, "deleted")
	h.broadcastAs(c, conf.WorkspaceID, "conference.deleted", gin.H{"id": conf.ID})
	c.Status(http.StatusNoContent)
}

// JoinConference records that the caller entered the room and flips a scheduled
// conference to live on the first arrival.
//
// It answers 409 for an ended conference rather than silently reopening it: a
// stale tab that reconnects an hour after the meeting finished must not put the
// call back on the board as live.
func (h *API) JoinConference(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	if conf.Status == "ended" {
		c.JSON(http.StatusConflict, gin.H{"error": "conference has ended"})
		return
	}
	uid := middleware.CurrentUser(c)
	// Someone who was never invited may still walk in — membership in the
	// workspace is the permission — and they join as a plain member. An invited
	// host keeps their role: the ON CONFLICT clause of the query leaves it alone.
	part, err := h.q.JoinConferenceParticipant(c, db.JoinConferenceParticipantParams{
		ConferenceID: conf.ID, UserID: uid, Role: "member",
	})
	if err != nil {
		fail(c, err)
		return
	}
	if conf.Status == "scheduled" {
		started, err := h.q.StartConference(c, conf.ID)
		switch {
		case err == nil:
			conf = started
			h.broadcast(conf.WorkspaceID, "conference.started", conf)
		case errors.Is(err, pgx.ErrNoRows):
			// Another participant won the race and started it a moment ago;
			// the returned zero rows are the expected outcome, not a failure.
			if refreshed, rerr := h.q.GetConference(c, conf.ID); rerr == nil {
				conf = refreshed
			}
		default:
			fail(c, err)
			return
		}
	}
	h.broadcast(conf.WorkspaceID, "conference.participant.joined", part)
	c.JSON(http.StatusOK, gin.H{"conference": conf, "participant": part})
}

// LeaveConference stamps the caller's exit and ends the conference when the
// room empties, so a call nobody is in stops showing as live on the board.
func (h *API) LeaveConference(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	uid := middleware.CurrentUser(c)
	part, err := h.q.LeaveConferenceParticipant(c, db.LeaveConferenceParticipantParams{
		ConferenceID: conf.ID, UserID: uid,
	})
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	h.broadcast(conf.WorkspaceID, "conference.participant.left", part)
	left, err := h.q.CountActiveConferenceParticipants(c, conf.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if left == 0 && conf.Status == "live" {
		ended, err := h.q.EndConference(c, conf.ID)
		if err == nil {
			conf = ended
			h.dropConfRoom(conf.ID, "ended")
			h.broadcast(conf.WorkspaceID, "conference.ended", conf)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			fail(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"conference": conf, "participant": part})
}

// EndConference closes a call for everyone. Moderators only — otherwise any
// participant could hang up the meeting on the rest of the room.
func (h *API) EndConference(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	if !h.requireConferenceManager(c, conf) {
		return
	}
	ended, err := h.q.EndConference(c, conf.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already ended — the desired state, so this is a success, not a 404.
		c.JSON(http.StatusOK, conf)
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	// Empty the live room too (#2869): everyone still connected is told the call
	// is over instead of sitting in a room whose conference has ended.
	h.dropConfRoom(conf.ID, "ended")
	h.broadcastAs(c, conf.WorkspaceID, "conference.ended", ended)
	c.JSON(http.StatusOK, ended)
}

// InviteConference adds workspace members to a conference. Invitees must be
// members of the same workspace: an invitation is also a permission to see the
// meeting, and inviting an outsider would hand them one.
//
// Each newly invited member is notified (#2875) — in-app, and through whatever
// external channels their routing rules point at. "Newly" is the operative word:
// the endpoint is idempotent by design (the dialog, a retry and a parallel
// /invite call all land here), so only a row that was not already an open
// invitation raises one.
func (h *API) InviteConference(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	var req struct {
		UserIDs []uuid.UUID `json:"user_ids" binding:"required"`
		Role    string      `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_ids must not be empty"})
		return
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	if role != "member" && role != "host" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	// Handing out a host seat is moderation: a member who could invite a
	// co-host could also invite themselves one on their next request.
	if role == "host" && !h.requireConferenceManager(c, conf) {
		return
	}
	invited := make([]db.ConferenceParticipant, 0, len(req.UserIDs))
	fresh := map[uuid.UUID]bool{}
	err := h.inTx(c, func(q *db.Queries) error {
		for _, uid := range req.UserIDs {
			if _, err := q.GetMembership(c, db.GetMembershipParams{
				WorkspaceID: conf.WorkspaceID, UserID: uid,
			}); err != nil {
				return err
			}
			// Read the seat before the upsert overwrites it: afterwards there is
			// no way to tell a first invitation from the third retry of one, and
			// notifying on every call would ring the same person repeatedly. A
			// member who had left counts as fresh — re-inviting them back into a
			// call they walked out of is a new invitation.
			prev, perr := q.GetConferenceParticipant(c, db.GetConferenceParticipantParams{
				ConferenceID: conf.ID, UserID: uid,
			})
			if errIsNoRows(perr) || (perr == nil && prev.LeftAt != nil) {
				fresh[uid] = true
			}
			p, err := q.InviteConferenceParticipant(c, db.InviteConferenceParticipantParams{
				ConferenceID: conf.ID, UserID: uid, Role: role,
			})
			if err != nil {
				return err
			}
			invited = append(invited, p)
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user is not a member of this workspace"})
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	msg := msgConferenceInvited(h.actorName(c), conf.Title, conf.ID.String())
	for _, p := range invited {
		h.broadcastAs(c, conf.WorkspaceID, "conference.participant.invited", p)
		if fresh[p.UserID] {
			// nil task: the notification points at the call, whose id travels in
			// the payload. notify() skips the inviter themselves.
			h.notify(c, p.UserID, conf.WorkspaceID, nil, kindConferenceInvite, msg)
		}
	}
	c.JSON(http.StatusOK, invited)
}

// ListConferenceParticipants returns everyone invited to a conference, with the
// attendance stamps that tell the panel who is in the room right now.
func (h *API) ListConferenceParticipants(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	rows, err := h.q.ListConferenceParticipants(c, conf.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, orEmpty(rows))
}
