package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"

	"tessera/internal/confroom"
	"tessera/internal/db"
	"tessera/internal/observability"
	"tessera/internal/realtime"
)

const (
	confWriteWait = 10 * time.Second
	confPongWait  = 60 * time.Second
	confPingEvery = 25 * time.Second
	// Every frame on this socket is a type plus at most a uuid and two booleans.
	// Media never comes through here, so the limit can stay small — it is what
	// keeps an open call from being a free memory-write primitive.
	confReadLimit = 512
)

// ConnectConference upgrades to the per-conference room socket (#2869):
// presence, hands, the screen-share queue and the moderation commands.
//
// It carries no media — that goes to the LiveKit SFU — and no chat bodies: a
// message posts over HTTP and the room only nudges everyone to refetch
// (#2873). What is left is the state the SFU has no opinion about, and the
// arbitration that has to have exactly one arbiter.
//
// Authorisation happens before the upgrade and answers three questions in
// order: is the token valid, does the conference exist, and is the caller a
// member of *its* workspace. A conference is reachable by id alone, so that
// last check is the only thing between a valid token and someone else's
// meeting.
func (h *WSHandler) ConnectConference(c *gin.Context) {
	uid, ok := h.authenticate(c)
	if !ok {
		// Answer before the upgrade so the client gets a real status code
		// instead of an opaque handshake failure.
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}
	confID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	conf, err := h.q.GetConference(c, confID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	role, err := h.q.GetMembershipRole(c, db.GetMembershipRoleParams{
		WorkspaceID: conf.WorkspaceID, UserID: uid,
	})
	if err != nil {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not a member of this workspace"})
		return
	}
	// An ended call is refused rather than reopened: a tab left open overnight
	// must not reconnect in the morning and show a room of ghosts.
	if conf.Status == "ended" {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "conference has ended"})
		return
	}
	if until, kicked := h.confRooms.KickedUntil(confID, uid); kicked {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "removed from this conference", "retry_after": until.UTC(),
		})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already wrote an error response.
	}
	seatRole, canModerate, forceMuted := h.conferenceSeat(c, conf, uid, role)
	p := confroom.NewParticipant(uid, h.displayName(c, uid), seatRole)
	p.CanModerate = canModerate
	room := h.confRooms.Join(confID, p, forceMuted)
	h.seedRecording(c, confID, room)
	go h.confWritePump(conn, p)
	go h.confReadPump(conn, p, room, confID)
}

// seedRecording puts the red dot back after the room has forgotten it (#2877).
//
// The room is in-memory and exists only while somebody is in it, so the first
// socket to arrive during a recording would otherwise see a snapshot that says
// nothing is being recorded — which is precisely the lie this indicator exists
// to prevent. Read after Join rather than before: a room that does not exist yet
// has nowhere to put the answer.
//
// A stale read (the recording ended between the query and here) cannot stick:
// Room.SetRecording refuses any recording the room has already seen finish.
func (h *WSHandler) seedRecording(c *gin.Context, confID uuid.UUID, room *confroom.Room) {
	rec, err := h.q.ActiveConferenceRecording(c, confID)
	if err != nil {
		return // no row is the ordinary case; a failed read is not worth a refused socket
	}
	view := confroom.RecordingView{ID: rec.ID.String(), StartedAt: rec.StartedAt}
	if rec.StartedByName != nil {
		view.StartedBy = *rec.StartedByName
	}
	room.SetRecording(&view)
}

// conferenceSeat resolves what this connection is allowed to do and what was
// already done to it: the role the room paints, whether this user may moderate,
// and whether a host has silenced them before.
//
// Role and moderation are two different things (#2878). The "host" label — with
// its kick-immunity and screen-share priority — belongs to the call's owner
// alone: the creator, or a participant explicitly enrolled as host. A workspace
// owner/admin may *moderate* any call (kick, force-mute) to match
// handlers.canManageConference, but that does not make them a host — they sit in
// the call as a plain member who happens to hold the moderation controls, and
// can be kicked like anyone else. Conflating the two is what made every admin
// show as "Ведущий" and left a two-admin call with nobody kickable.
//
// Resolved once, at join: a role change mid-call takes effect on the next
// connection, which is what makes the room's own checks cheap.
//
// The force-mute flag has to be read here because the room forgets it when the
// last person leaves (#2872). Without this a muted participant would only have
// to wait out the call and rejoin first to have their microphone back.
func (h *WSHandler) conferenceSeat(c *gin.Context, conf db.Conference, uid uuid.UUID, wsRole string) (role string, canModerate, forceMuted bool) {
	role = confroom.RoleMember
	isCreator := conf.CreatedBy != nil && *conf.CreatedBy == uid
	if isCreator {
		role = confroom.RoleHost
	}
	// Moderation, unlike the host label, follows the workspace manager rights.
	canModerate = isCreator || wsRole == "owner" || wsRole == "admin"
	part, err := h.q.GetConferenceParticipant(c, db.GetConferenceParticipantParams{
		ConferenceID: conf.ID, UserID: uid,
	})
	if err != nil {
		// No row yet — a member walking into an open call before the join request
		// lands. They cannot have been muted, and the seat above stands.
		return role, canModerate, false
	}
	// An explicitly enrolled host is the call's owner too: they get the label and
	// the moderation rights that come with it.
	if part.Role == confroom.RoleHost {
		role = confroom.RoleHost
		canModerate = true
	}
	return role, canModerate, part.ForceMuted
}

// confReadPump handles room commands until the socket dies, then takes the
// participant out — which is what frees its place on the stage immediately
// rather than at the TTL.
func (h *WSHandler) confReadPump(conn *websocket.Conn, p *confroom.Participant, room *confroom.Room, confID uuid.UUID) {
	// Per-connection goroutine: a panic here would crash the server, so report
	// it and let the deferred cleanup below still run.
	defer observability.Recover("conf-ws-read")
	defer func() {
		h.confRooms.Leave(confID, p)
		_ = conn.Close()
		// Reconcile the durable roster with the dropped socket (#2864): a user who
		// navigated to another section — or closed the tab — without pressing
		// «Выйти» must stop counting as present, or the call never pauses and
		// everyone else keeps seeing them in the room.
		h.reconcileConferenceLeave(confID, p.UserID)
	}()
	conn.SetReadLimit(confReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(confPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(confPongWait))
	})
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg struct {
			Type   string `json:"type"`
			Mic    bool   `json:"mic"`
			Cam    bool   `json:"cam"`
			Up     bool   `json:"up"`
			Muted  bool   `json:"muted"`
			UserID string `json:"user_id"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue // malformed frame; not worth dropping the connection over
		}
		now := time.Now()
		switch msg.Type {
		case confroom.TypeMedia:
			room.SetMedia(p, msg.Mic, msg.Cam)
		case confroom.TypeHand:
			room.RaiseHand(p, msg.Up, now)
		case confroom.TypeScreenRequest:
			room.RequestScreen(p, now)
		case confroom.TypeScreenRefresh:
			room.RefreshScreen(p, now)
		case confroom.TypeScreenRelease:
			room.ReleaseScreen(p, now)
		case confroom.TypeKick:
			if target, err := uuid.Parse(msg.UserID); err == nil {
				room.Kick(p, target, now)
			}
		case confroom.TypeMute:
			if target, err := uuid.Parse(msg.UserID); err == nil {
				room.SetMuted(p, target, msg.Muted)
			}
		}
	}
}

// reconcileConferenceLeave stamps a user's exit in the database when their last
// socket to a conference drops, and pauses the call if the room is now empty.
// This is the server-authoritative counterpart to the REST /leave: a client that
// simply navigates away never calls it, so without this the participant row keeps
// claiming they are present and the conference is stuck «live» forever.
//
// It runs off the read pump's deferred cleanup, detached from any request, so it
// uses a background context and reports nothing back to the (already gone) socket.
func (h *WSHandler) reconcileConferenceLeave(confID, userID uuid.UUID) {
	// Another tab of the same user still open → they are still in the call.
	if h.confRooms.UserPresent(confID, userID) {
		return
	}
	ctx := context.Background()
	conf, err := h.q.GetConference(ctx, confID)
	if err != nil {
		return
	}
	part, err := h.q.LeaveConferenceParticipant(ctx, db.LeaveConferenceParticipantParams{
		ConferenceID: confID, UserID: userID,
	})
	if err != nil {
		// No participant row (a member who opened the socket before the join
		// request landed, then left) — nothing durable to reconcile.
		return
	}
	h.broadcastConf(conf.WorkspaceID, "conference.participant.left", part)
	left, err := h.q.CountActiveConferenceParticipants(ctx, confID)
	if err != nil {
		return
	}
	if left == 0 && conf.Status == "live" {
		paused, perr := h.q.PauseConference(ctx, confID)
		if perr == nil {
			h.broadcastConf(conf.WorkspaceID, "conference.ended", paused)
		} else if !errors.Is(perr, pgx.ErrNoRows) {
			observability.CaptureError("conf-reconcile-pause", perr)
		}
	}
}

// broadcastConf fans a conference event out to a workspace over the hub. Used
// from the socket layer, which has no gin.Context and so no acting user.
func (h *WSHandler) broadcastConf(workspaceID uuid.UUID, eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.hub.Broadcast(realtime.Event{Scope: workspaceID.String(), Type: eventType, Data: data})
}

// confWritePump writes room snapshots and keeps the socket alive through
// proxies.
func (h *WSHandler) confWritePump(conn *websocket.Conn, p *confroom.Participant) {
	defer observability.Recover("conf-ws-write")
	ticker := time.NewTicker(confPingEvery)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()
	for {
		select {
		case frame, ok := <-p.Out():
			_ = conn.SetWriteDeadline(time.Now().Add(confWriteWait))
			if !ok {
				// Evicted as too slow, kicked, or the process is shutting down. A
				// close frame makes the client reconnect and re-read the room
				// instead of sitting on a stale snapshot.
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(confWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
