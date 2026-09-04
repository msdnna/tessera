package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/confroom"
	"tessera/internal/db"
	"tessera/internal/livekit"
	"tessera/internal/observability"
)

// Conference moderation (#2864, subtask #2872): the half of a kick or a
// force-mute that lives outside the room's memory.
//
// internal/confroom decides *whether* a moderation command is allowed — it is
// the only place that knows who is a host — and then hands the decision here.
// This file is what makes it stick:
//
//   - the database, so the decision survives the room. A force-mute held only in
//     memory is undone by everyone leaving and one person rejoining, and a kick
//     would leave the participant row still reading "in the call".
//   - the SFU, so it applies to the audio. LiveKit takes orders from our service
//     token and has no idea who asked for what; a client that ignores the muted
//     flag keeps publishing until RoomService is told otherwise.
//
// The seam exists because confroom must not import either one: it is the
// arbiter of live state, and a package that reaches for a database connection
// while holding a room's mutex is one slow query away from stalling every call
// on the server.

// confModerationTimeout bounds one enforcement round. Generous compared to a
// request handler — nobody is waiting on it — but finite, so a wedged SFU leaks
// a goroutine for ten seconds rather than for the life of the process.
const confModerationTimeout = 10 * time.Second

// confEnforcer implements confroom.Enforcer against this API's dependencies.
type confEnforcer struct{ h *API }

// ConfEnforcer returns the sink to install with confroom.Rooms.SetEnforcer.
// Exported because the wiring lives in router.go, next to where the registry is
// created.
func (h *API) ConfEnforcer() confroom.Enforcer { return confEnforcer{h: h} }

// dispatch runs one enforcement in the background.
//
// Background rather than inline because the caller is a socket's read pump: a
// host whose kick is waiting on the SFU would stop receiving room snapshots
// mid-command, and their own UI would freeze on the action they just took. The
// context is detached from the request for the same reason — the command that
// triggered this has already been answered.
//
// op names the enforcement in the panic report: a goroutine that dies here dies
// with no request around it, so the component label is the only thing that says
// whether a kick or a force-mute was to blame.
func (e confEnforcer) dispatch(op string, fn func(ctx context.Context)) {
	go func() {
		defer observability.Recover("conf-moderation." + op)
		ctx, cancel := context.WithTimeout(context.Background(), confModerationTimeout)
		defer cancel()
		fn(ctx)
	}()
}

// Kicked records that a user was removed from a call and disconnects them from
// the SFU.
//
// Note what is *not* here: ending the conference when the room empties. A kick
// is issued by a host who is themselves in the call, so it can never remove the
// last participant — the emptying path stays where it belongs, in
// LeaveConference.
func (e confEnforcer) Kicked(confID, userID uuid.UUID) {
	e.dispatch("kick", func(ctx context.Context) {
		part, err := e.h.q.LeaveConferenceParticipant(ctx, db.LeaveConferenceParticipantParams{
			ConferenceID: confID, UserID: userID,
		})
		switch {
		case err == nil:
			// The board's conference list counts who is in the room, so it has to
			// hear about an exit nobody requested over HTTP.
			if wsID, werr := e.h.q.WorkspaceIDForConference(ctx, confID); werr == nil {
				e.h.broadcast(wsID, "conference.participant.left", part)
			}
		case errors.Is(err, pgx.ErrNoRows):
			// Someone who walked in without a participant row — possible while a
			// join is still in flight. There is nothing to stamp, and the removal
			// below is still worth doing.
		default:
			soft(ctx, "conference.kick.persist", err)
		}
		// NotFound is the ordinary case, not a failure: the kicked client usually
		// drops its own media connection the moment the room closes its socket,
		// and by the time this runs LiveKit has already forgotten them.
		if err := e.h.livekit.RemoveParticipant(ctx, livekit.RoomName(confID), userID.String()); err != nil {
			var lkErr *livekit.Error
			gone := errors.As(err, &lkErr) && lkErr.NotFound()
			if !errors.Is(err, livekit.ErrDisabled) && !gone {
				soft(ctx, "conference.kick.sfu", err)
			}
		}
	})
}

// Muted persists a force-mute and carries it to the SFU.
//
// Order matters on the way in: the publish permission is narrowed *before* the
// live track is silenced. Muting first would leave a window in which the client
// notices the mute and publishes a fresh microphone track, which would then be
// perfectly legal — narrowing first closes it.
//
// On the way out only the permission is restored. LiveKit will not remotely
// unmute a track anyway (room.enable_remote_unmute stays off, deliberately), and
// switching somebody's microphone on from the server is not moderation.
func (e confEnforcer) Muted(confID, userID uuid.UUID, muted bool) {
	e.dispatch("mute", func(ctx context.Context) {
		if _, err := e.h.q.SetConferenceParticipantMuted(ctx, db.SetConferenceParticipantMutedParams{
			ConferenceID: confID, UserID: userID, ForceMuted: muted,
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			soft(ctx, "conference.mute.persist", err)
		}
		if !e.h.livekit.Enabled() {
			return
		}
		room, identity := livekit.RoomName(confID), userID.String()
		sources := livekit.AllSources()
		if muted {
			sources = livekit.SourcesWithoutMic()
		}
		if err := e.h.livekit.SetPublishSources(ctx, room, identity, sources); err != nil {
			soft(ctx, "conference.mute.permission", err)
			// Without the permission change the mute below is cosmetic — the client
			// may republish at once — but silencing what is live is still strictly
			// better than doing nothing, so this is not a return.
		}
		if !muted {
			return
		}
		soft(ctx, "conference.mute.tracks", e.muteMicTracks(ctx, room, identity))
	})
}

// muteMicTracks silences every microphone track the participant currently
// publishes. Plural on purpose: a second tab is a second published track, and
// muting only the first would leave the room hearing them from the other one.
func (e confEnforcer) muteMicTracks(ctx context.Context, room, identity string) error {
	people, err := e.h.livekit.ListParticipants(ctx, room)
	if err != nil {
		var lkErr *livekit.Error
		if errors.As(err, &lkErr) && lkErr.NotFound() {
			return nil // nobody has connected media to this conference yet
		}
		return err
	}
	for _, p := range people {
		if p.Identity != identity {
			continue
		}
		for _, tr := range p.Tracks {
			// Source is the reliable discriminator: Type AUDIO also covers the
			// audio half of a screen share, and killing that would turn "mute this
			// person" into "break the presentation they are giving".
			if tr.Source != livekit.SourceMicrophone || tr.Muted {
				continue
			}
			if err := e.h.livekit.MuteTrack(ctx, room, identity, tr.SID); err != nil {
				return err
			}
		}
	}
	return nil
}
