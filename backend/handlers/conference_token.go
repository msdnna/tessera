package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/livekit"
	"tessera/middleware"
)

// Conference media tokens (#2864, subtask #2871).
//
// This is the single seam between our permission model and the SFU. LiveKit has
// no idea who a Tessera user is and must not learn: all it can do is verify a
// signature. So the token issued here is a narrow, short-lived warrant —
// "this identity may join this one room" — minted only after our own checks
// have passed, and never carrying the administrative grants (see livekit.Grants).
//
// The endpoint is also where room creation happens. livekit-server runs with
// room.auto_create=false on purpose (deploy/livekit.yaml): without that, a valid
// token for a room nobody made would silently create it, which is a free
// anonymous video host on our server. Creating it here — after authorization —
// keeps the room set equal to the conference set.

// TokenResponse is what the browser needs to reach the SFU: where to connect and
// what to present. The API key and secret stay on the server; the client only
// ever sees a signed token.
type TokenResponse struct {
	// URL is the signalling address (wss://…/livekit), which goes through Caddy
	// on 443 — not the compose-internal address the backend itself uses.
	URL string `json:"url"`
	// Token is the join credential. Short-lived: see livekit.TokenTTL.
	Token string `json:"token"`
	// Room and Identity are echoed so the client can assert the connection it
	// ends up with is the one it asked for, instead of trusting its own guess at
	// the naming scheme.
	Room     string `json:"room"`
	Identity string `json:"identity"`
	// ExpiresIn is the token's life in seconds — the client refreshes on a
	// reconnect rather than caching it for the length of the meeting.
	ExpiresIn int `json:"expires_in"`
}

// ConferenceToken issues a LiveKit join token for the caller.
//
// Refuses in three cases that all mean "there is no room to join": the SFU is
// not configured for this install, the conference has ended, or the caller is
// serving a kick cooldown. That last check matters more than it looks — a kick
// removes someone from our presence room, and if the media token were still
// issued they would keep talking to everyone from outside the roster.
func (h *API) ConferenceToken(c *gin.Context) {
	conf, ok := h.conferenceScope(c)
	if !ok {
		return
	}
	// PublicURL is checked alongside Enabled because a token with nowhere to
	// spend it is not a working conference — better a clear 503 here than a
	// browser failing to connect to the empty string.
	if !h.livekit.Enabled() || h.livekit.PublicURL() == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "conferences are not configured on this server"})
		return
	}
	if conf.Status == "ended" {
		c.JSON(http.StatusConflict, gin.H{"error": "conference has ended"})
		return
	}
	uid := middleware.CurrentUser(c)
	if until, kicked := h.confRooms.KickedUntil(conf.ID, uid); kicked {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "removed from this conference", "retry_after": until.UTC(),
		})
		return
	}

	room := livekit.RoomName(conf.ID)
	// Safe to repeat on every join: LiveKit returns the existing room rather
	// than erroring, so there is no create-once state to keep on our side.
	if _, err := h.livekit.CreateRoom(c, room, livekit.RoomOptions{}); err != nil {
		fail(c, err)
		return
	}
	// Everyone in a Tessera conference is a full participant, with one exception:
	// a host has taken this one's microphone away.
	sources := []string(nil)
	if h.conferenceForceMuted(c, conf.ID, uid) {
		sources = livekit.SourcesWithoutMic()
	}
	token, err := h.livekit.JoinToken(uid.String(), h.userDisplayName(c, uid), livekit.Grants{
		Room: room,
		// An ordinary mute is a room-state decision (#2869) and does not belong in
		// the grant, which the client could otherwise simply re-request. A
		// force-mute does, for the opposite reason — see conferenceForceMuted.
		CanPublish:        true,
		CanSubscribe:      true,
		CanPublishData:    true,
		CanPublishSources: sources,
	}, 0)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, TokenResponse{
		URL:       h.livekit.PublicURL(),
		Token:     token,
		Room:      room,
		Identity:  uid.String(),
		ExpiresIn: int(livekit.TokenTTL.Seconds()),
	})
}

// conferenceForceMuted reports whether a host has silenced this user in this
// conference — the live room's word first, the participant row as the fallback.
//
// Why the token has to carry it (#2932). A force-mute is enforced at the SFU by
// narrowing the participant's publish sources through RoomService, and that
// narrowing lives on the participant *session*: LiveKit authorises a join once,
// from the token, and a reconnecting client is a brand new participant built
// from a brand new token. So the enforcement the moderation path installs is
// undone by anything that re-joins the media — a dropped connection, a page
// reload, a second tab — while our own side still shows the person as muted,
// because that half *is* durable (it is on their row, and conferenceSeat seeds
// it back into the room). What came out was the failure the whole moderation
// file exists to prevent: the roster says silenced, the microphone is live
// again, and only a client that ignores the flag has to notice.
//
// Two sources, because neither alone covers the join: the room holds the flag
// the moment a host sets it, while the row is written by a background enforcer
// (a token minted in that window would miss it) and is all that is left once the
// last person leaves and the room is forgotten.
//
// A read failure is not a refusal — the ordinary case for "no row" is a member
// opening the call before their join lands, and nobody may be muted yet.
func (h *API) conferenceForceMuted(c *gin.Context, confID, uid uuid.UUID) bool {
	if h.confRooms != nil && h.confRooms.UserMuted(confID, uid) {
		return true
	}
	part, err := h.q.GetConferenceParticipant(c, db.GetConferenceParticipantParams{
		ConferenceID: confID, UserID: uid,
	})
	return err == nil && part.ForceMuted
}

// userDisplayName resolves the label LiveKit shows for a participant. Mirrors
// WSHandler.displayName: the same person must not be "Иван" in the presence
// roster and "ivan" on their video tile.
func (h *API) userDisplayName(c *gin.Context, uid uuid.UUID) string {
	u, err := h.q.GetUserByID(c, uid)
	if err != nil {
		return "Участник"
	}
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	if local, _, ok := strings.Cut(u.Email, "@"); ok && local != "" {
		return local
	}
	return "Участник"
}
