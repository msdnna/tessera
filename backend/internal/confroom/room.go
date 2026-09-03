// Package confroom is the live state of one conference: who is actually in the
// call right now, who has their hand up, who holds the screen-share stage and
// who is next in line for it.
//
// It is deliberately *not* the media path. Audio and video go to the LiveKit
// SFU (#2866/#2867) and never pass through this process; what is left over is
// everything the SFU has no opinion about — the room's own bookkeeping. That
// split is what keeps the permission model in one place: LiveKit only verifies
// a token signature, so "may this person kick that one" has to be answered
// here.
//
// The shape is lifted from internal/docroom, for the same reasons spelled out
// there: internal/realtime's hub is one-way (its clients only consume
// broadcasts, while a stage has to be *asked* for and answered) and
// workspace-scoped (room traffic would fan out to everyone in the workspace,
// most of whom are not in the call). Two properties carry over and are
// load-bearing:
//
//   - every broadcast carries the *whole* room state rather than a delta, so a
//     reconnect is self-healing;
//   - a participant whose buffer overflows is evicted rather than silently
//     skipped — a dropped frame would leave the stage shown as held by someone
//     who left ten minutes ago, and nothing would ever correct it.
//
// The durable side of moderation is not here either, but it is no longer
// missing: a decision taken in this package is handed to an Enforcer (#2872),
// which persists it to conference_participants and carries it to the SFU. That
// indirection is what keeps this file free of both the database and LiveKit
// while still making a force-mute silence a real microphone rather than only
// the badge above it.
package confroom

import (
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// RoleHost may kick and force-mute; RoleMember may not. This is the role
	// *inside the call*, which the socket handler derives from the conference's
	// participant row and the workspace role — the room itself never queries the
	// database.
	RoleHost = "host"
	// RoleMember is everyone else in the call — the default, and what an
	// unrecognised role degrades to.
	RoleMember = "member"

	// StageTTL is how long the screen-share stage survives without a refresh.
	// The presenter's client re-sends screen.refresh while it is sharing, so the
	// TTL only matters when a client dies without closing its socket. Without it
	// a laptop lid closing would park the stage forever and the queue behind it.
	StageTTL = 30 * time.Second

	// SweepEvery is how often an abandoned stage is collected. The stage is also
	// checked lazily on every request, so this only bounds how long an
	// *unobserved* stale presenter stays on screen.
	SweepEvery = 5 * time.Second

	// KickCooldown is how long a kicked user is kept out of the presence room and
	// refused a media token. The durable half — clearing the participant row and
	// removing them from the SFU — is the Enforcer's job; this is what keeps them
	// from simply walking back in while it happens.
	KickCooldown = 5 * time.Minute

	// sendBuffer is how many frames may queue for one participant before it is
	// evicted as too slow. Snapshots are small and idempotent; a client that
	// cannot keep up with this is not one we can usefully talk to.
	sendBuffer = 32
)

// Message types on the wire.
//
// Server→client: welcome (once, on join), state (whole-room snapshot), denied
// (a command was refused, with a reason), chat (something was said — refetch
// the thread, #2873), ended (the call is over; the room is about to close).
//
// Client→server: media (my microphone/camera state changed), hand (raise or
// lower), screen.request / screen.refresh / screen.release (the stage),
// kick and mute (moderation; refused for a non-host).
const (
	TypeWelcome = "welcome"
	TypeState   = "state"
	TypeDenied  = "denied"
	TypeChat    = "chat"
	TypeEnded   = "ended"

	TypeMedia         = "media"
	TypeHand          = "hand"
	TypeScreenRequest = "screen.request"
	TypeScreenRefresh = "screen.refresh"
	TypeScreenRelease = "screen.release"
	TypeKick          = "kick"
	TypeMute          = "mute"
)

// Participant is one open socket in one conference. Identity here is the
// connection, not the user: the same person may have the call open in two tabs,
// and the second tab must not inherit the first one's stage.
type Participant struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Name   string
	Role   string
	// CanModerate is the right to kick and force-mute, kept apart from Role on
	// purpose (#2878): a workspace admin may moderate a call without being its
	// host. Role is the label the room paints and the kick-immunity that protects
	// a host; CanModerate is only "may issue moderation commands". Set once before
	// Join, then read under the room lock like the fields below.
	CanModerate bool

	// Guarded by the room's mutex, not by the participant: every read of these
	// happens while building a snapshot, which already holds it.
	mic      bool
	cam      bool
	handAt   time.Time // zero means the hand is down
	joinedAt time.Time

	send   chan []byte
	closed chan struct{}
	once   sync.Once
}

// NewParticipant returns a participant ready to be joined to a room. An
// unrecognised role is read as a plain member — a typo must not hand out
// moderation rights.
func NewParticipant(userID uuid.UUID, name, role string) *Participant {
	if role != RoleHost {
		role = RoleMember
	}
	return &Participant{
		ID:     uuid.New(),
		UserID: userID,
		Name:   name,
		Role:   role,
		// A host moderates by default; the admin-who-is-a-member case is the one
		// the caller opts into by flipping CanModerate on afterwards (#2878).
		CanModerate: role == RoleHost,
		send:        make(chan []byte, sendBuffer),
		closed:      make(chan struct{}),
	}
}

// Out is the stream of frames to write to the socket. It is closed when the
// participant leaves or is evicted, which is the write pump's signal to finish.
func (p *Participant) Out() <-chan []byte { return p.send }

// Close releases the participant. Safe to call repeatedly and from either side
// (the socket noticing a dead peer, or the room evicting a slow one).
func (p *Participant) Close() {
	p.once.Do(func() {
		close(p.closed)
		close(p.send)
	})
}

// deliver queues a frame, evicting the participant if its buffer is full. The
// caller must hold the room lock; eviction only closes channels, so no reader
// can re-enter the room from here.
func (p *Participant) deliver(frame []byte) {
	select {
	case <-p.closed:
		return
	default:
	}
	select {
	case p.send <- frame:
	default:
		log.Printf("confroom: evicting slow participant %s", p.ID)
		p.Close()
	}
}

// stage is the screen-share slot. It holds the participant pointer rather than
// its id because a host preempting a member has to put that member back at the
// head of the queue, and a queue of ids could not be re-entered.
type stage struct {
	p       *Participant
	since   time.Time
	expires time.Time
}

// Enforcer carries a moderation decision out of this process: to the database,
// so it survives a reconnect, and to the SFU, so it applies to the media stream
// this package deliberately knows nothing about.
//
// Both methods are called *after* the room's lock is released and must not
// block the caller — they run on a socket's read pump, and a wedged SFU must not
// be able to freeze a participant's connection. The implementation owns its own
// goroutine and timeout.
//
// A nil Enforcer is legal and means "in-memory only": every test that exercises
// arbitration, and any install without an SFU, runs that way.
type Enforcer interface {
	// Kicked is called once a kick has taken effect in the room.
	Kicked(confID, userID uuid.UUID)
	// Muted is called when a user's force-mute flag was flipped either way.
	Muted(confID, userID uuid.UUID, muted bool)
}

// Room is the live state of one conference.
type Room struct {
	confID  uuid.UUID
	enforce Enforcer

	mu      sync.Mutex
	members map[*Participant]struct{}
	// force-mute is per user, not per connection: muting someone must not leave
	// their second tab talking.
	muted  map[uuid.UUID]bool
	kicked map[uuid.UUID]time.Time // user id -> cooldown expiry
	stage  *stage
	queue  []*Participant
}

func newRoom(confID uuid.UUID, enforce Enforcer) *Room {
	return &Room{
		confID:  confID,
		enforce: enforce,
		members: map[*Participant]struct{}{},
		muted:   map[uuid.UUID]bool{},
		kicked:  map[uuid.UUID]time.Time{},
	}
}

// PersonView is one person in the call. Conns counts their open sockets, so the
// UI shows a single tile for someone who joined from two tabs; mic and cam are
// true when *any* of those sockets has the device live.
type PersonView struct {
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Role       string     `json:"role"`
	Conns      int        `json:"conns"`
	Mic        bool       `json:"mic"`
	Cam        bool       `json:"cam"`
	ForceMuted bool       `json:"force_muted"`
	HandAt     *time.Time `json:"hand_at"`
	JoinedAt   time.Time  `json:"joined_at"`
}

// StageView is the current presenter as clients see it.
type StageView struct {
	UserID string    `json:"user_id"`
	ConnID string    `json:"conn_id"`
	Name   string    `json:"name"`
	Since  time.Time `json:"since"`
}

// QueueView is one waiting request for the stage, in the order it will be
// served.
type QueueView struct {
	UserID string `json:"user_id"`
	ConnID string `json:"conn_id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// StateMsg is the whole-room snapshot. Sending everything on every change
// (rather than add/remove deltas) is what makes a reconnect enough to recover.
type StateMsg struct {
	Type         string       `json:"type"`
	Participants []PersonView `json:"participants"`
	Stage        *StageView   `json:"stage"`
	Queue        []QueueView  `json:"queue"`
}

// welcomeMsg tells a fresh participant which connection it is. A client needs
// this to tell "I am presenting" from "my other tab is presenting", which is
// the difference between a stop-sharing button and a lie.
type welcomeMsg struct {
	Type         string `json:"type"`
	ConnID       string `json:"conn_id"`
	UserID       string `json:"user_id"`
	Role         string `json:"role"`
	// CanModerate travels alongside Role so the client shows the kick/force-mute
	// controls to a workspace admin who is a plain member of the call (#2878) —
	// Role alone would hide them.
	CanModerate  bool   `json:"can_moderate"`
	ConferenceID string `json:"conference_id"`
	StageTTLMs   int64  `json:"stage_ttl_ms"`
}

// deniedMsg answers a command the room refused. Action names the command so the
// client can put the message where it belongs instead of showing a floating
// error with no context.
type deniedMsg struct {
	Type   string `json:"type"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// join adds a participant, welcomes it and broadcasts the new state.
//
// forceMuted is the flag as stored on the participant's row. Seeding it here is
// what makes a force-mute survive an empty room: the last person leaving throws
// the in-memory flag away with it, and without this the muted participant would
// get their microphone back simply by being the first to rejoin.
func (r *Room) join(p *Participant, forceMuted bool, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.joinedAt = now
	if forceMuted {
		r.muted[p.UserID] = true
	}
	r.members[p] = struct{}{}
	p.deliver(encode(welcomeMsg{
		Type:         TypeWelcome,
		ConnID:       p.ID.String(),
		UserID:       p.UserID.String(),
		Role:         p.Role,
		CanModerate:  p.CanModerate,
		ConferenceID: r.confID.String(),
		StageTTLMs:   StageTTL.Milliseconds(),
	}))
	r.broadcastLocked()
}

// leave removes a participant, releasing the stage and its place in the queue.
// Releasing on disconnect (rather than waiting for the TTL) is what makes
// closing a tab feel immediate to the rest of the room.
func (r *Room) leave(p *Participant, now time.Time) (empty bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[p]; !ok {
		return len(r.members) == 0
	}
	r.removeLocked(p, now)
	r.broadcastLocked()
	return len(r.members) == 0
}

// removeLocked takes a participant out of the room, the queue and the stage,
// promoting the next presenter if it held one. Caller holds r.mu.
func (r *Room) removeLocked(p *Participant, now time.Time) {
	delete(r.members, p)
	r.dequeueLocked(p)
	if r.stage != nil && r.stage.p == p {
		r.promoteLocked(now)
	}
	p.Close()
}

// SetMedia records whether this connection currently has its microphone and
// camera live. The room is not the source of truth for the tracks themselves —
// LiveKit is — but the tile grid has to paint a muted badge before the first
// audio packet arrives, and a force-muted participant must not be able to
// report a live microphone.
func (r *Room) SetMedia(p *Participant, mic, cam bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.muted[p.UserID] {
		mic = false
	}
	if p.mic == mic && p.cam == cam {
		return
	}
	p.mic, p.cam = mic, cam
	r.broadcastLocked()
}

// RaiseHand puts a hand up or down. The timestamp is what orders the "wants to
// speak" list; re-raising an already raised hand keeps the original time so a
// reconnect does not send someone to the back of the line.
func (r *Room) RaiseHand(p *Participant, up bool, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case up && p.handAt.IsZero():
		p.handAt = now
	case !up && !p.handAt.IsZero():
		p.handAt = time.Time{}
	default:
		return
	}
	r.broadcastLocked()
}

// RequestScreen asks for the screen-share stage. It reports whether the caller
// is presenting *now*; a false answer means the request was queued, and the
// snapshot that follows says where in the line.
//
// The arbitration rules, in the order they are applied:
//
//   - the stage is free                → take it;
//   - the caller already holds it      → refresh (this is the heartbeat);
//   - a host asks, a member presents   → the host preempts, and the member goes
//     back to the *head* of the queue: they were interrupted, not passed over;
//   - anything else                    → queue, hosts ahead of members.
//
// A host does not preempt another host: two moderators fighting over the stage
// is worse than one of them waiting.
func (r *Room) RequestScreen(p *Participant, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireStageLocked(now)
	if r.stage != nil && r.stage.p == p {
		r.stage.expires = now.Add(StageTTL)
		return true
	}
	if r.stage == nil {
		r.dequeueLocked(p)
		r.takeStageLocked(p, now)
		r.broadcastLocked()
		return true
	}
	if p.Role == RoleHost && r.stage.p.Role != RoleHost {
		preempted := r.stage.p
		r.dequeueLocked(p)
		r.takeStageLocked(p, now)
		r.queue = append([]*Participant{preempted}, r.queue...)
		r.broadcastLocked()
		return true
	}
	if r.enqueueLocked(p) {
		r.broadcastLocked()
	}
	return false
}

// RefreshScreen extends the presenter's hold. Anyone else calling it is
// ignored: a stale client must not be able to keep someone else's stage alive.
func (r *Room) RefreshScreen(p *Participant, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stage == nil || r.stage.p != p {
		return
	}
	r.stage.expires = now.Add(StageTTL)
}

// ReleaseScreen gives the stage up and hands it to whoever is next. Called by
// someone who is not presenting it only drops them out of the queue — which is
// exactly what "never mind" should do.
func (r *Room) ReleaseScreen(p *Participant, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stage != nil && r.stage.p == p {
		r.promoteLocked(now)
		r.broadcastLocked()
		return
	}
	if r.dequeueLocked(p) {
		r.broadcastLocked()
	}
}

// Kick removes every connection of a user from the room and keeps them out for
// KickCooldown. Moderators only (a host or a workspace admin, #2878); a refusal
// is answered to the caller rather than silently ignored, so the UI can say why
// the button did nothing.
//
// Kicking a host is refused: the host is the call's owner and ejecting them is
// how a meeting ends up with nobody able to run it. A moderator who is only an
// admin (Role member) is not protected — they can be kicked like anyone else.
func (r *Room) Kick(actor *Participant, target uuid.UUID, now time.Time) bool {
	if !r.kick(actor, target, now) {
		return false
	}
	// Outside the lock on purpose: the enforcer talks to Postgres and the SFU,
	// and holding the room's mutex across either would stall every other
	// participant's snapshot behind a network round trip.
	if r.enforce != nil {
		r.enforce.Kicked(r.confID, target)
	}
	return true
}

// kick applies the kick to the room's own state, reporting whether it took.
func (r *Room) kick(actor *Participant, target uuid.UUID, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !actor.CanModerate {
		actor.deliver(encode(deniedMsg{Type: TypeDenied, Action: TypeKick, Reason: "not a moderator"}))
		return false
	}
	if target == actor.UserID {
		actor.deliver(encode(deniedMsg{Type: TypeDenied, Action: TypeKick, Reason: "cannot kick yourself"}))
		return false
	}
	victims := make([]*Participant, 0, 2)
	for m := range r.members {
		if m.UserID != target {
			continue
		}
		if m.Role == RoleHost {
			actor.deliver(encode(deniedMsg{Type: TypeDenied, Action: TypeKick, Reason: "cannot kick a host"}))
			return false
		}
		victims = append(victims, m)
	}
	r.kicked[target] = now.Add(KickCooldown)
	for _, m := range victims {
		// The close frame is queued before the channel is closed, so the write
		// pump still delivers it: a client that just goes quiet cannot tell a
		// kick from a network drop, and would reconnect in a loop.
		m.deliver(encode(struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		}{TypeEnded, "kicked"}))
		r.removeLocked(m, now)
	}
	r.broadcastLocked()
	return true
}

// SetMuted force-mutes or unmutes a user for everyone. Moderators only (a host
// or a workspace admin, #2878).
//
// The flag decides what this room will accept in SetMedia and what the roster
// paints; the Enforcer is what makes it true of the microphone itself, by
// muting the published track and narrowing the participant's publish
// permission at the SFU. Both halves are needed: without the flag the UI lies
// about state it cannot see, and without the enforcement a modified client
// keeps talking.
//
// Unmuting is not the mirror image. It restores the permission to publish a
// microphone, and stops there — turning somebody's microphone back on from the
// server is not moderation, it is eavesdropping, so the participant has to
// unmute themselves.
func (r *Room) SetMuted(actor *Participant, target uuid.UUID, muted bool) bool {
	changed, ok := r.setMuted(actor, target, muted)
	if ok && changed && r.enforce != nil {
		r.enforce.Muted(r.confID, target, muted)
	}
	return ok
}

// setMuted flips the flag in the room, reporting whether it was allowed and
// whether anything actually changed.
func (r *Room) setMuted(actor *Participant, target uuid.UUID, muted bool) (changed, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !actor.CanModerate {
		actor.deliver(encode(deniedMsg{Type: TypeDenied, Action: TypeMute, Reason: "not a moderator"}))
		return false, false
	}
	if r.muted[target] == muted {
		return false, true
	}
	if muted {
		r.muted[target] = true
		for m := range r.members {
			if m.UserID == target {
				m.mic = false
			}
		}
	} else {
		delete(r.muted, target)
	}
	r.broadcastLocked()
	return true, true
}

// Muted reports whether a user is force-muted in this room.
func (r *Room) Muted(userID uuid.UUID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.muted[userID]
}

// kickedUntil reports the remaining cooldown for a user, if any.
func (r *Room) kickedUntil(userID uuid.UUID, now time.Time) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	until, ok := r.kicked[userID]
	if !ok || !until.After(now) {
		return time.Time{}, false
	}
	return until, true
}

// takeStageLocked puts p on the stage. Caller holds r.mu.
func (r *Room) takeStageLocked(p *Participant, now time.Time) {
	r.stage = &stage{p: p, since: now, expires: now.Add(StageTTL)}
}

// promoteLocked frees the stage and gives it to the head of the queue.
// Caller holds r.mu.
func (r *Room) promoteLocked(now time.Time) {
	r.stage = nil
	for len(r.queue) > 0 {
		next := r.queue[0]
		r.queue = r.queue[1:]
		// Someone may have left while waiting; skip them rather than parking the
		// stage on a closed socket.
		if _, ok := r.members[next]; ok {
			r.takeStageLocked(next, now)
			return
		}
	}
}

// enqueueLocked adds p to the waiting line, hosts ahead of members, and reports
// whether anything changed. Caller holds r.mu.
func (r *Room) enqueueLocked(p *Participant) bool {
	for _, q := range r.queue {
		if q == p {
			return false
		}
	}
	if p.Role != RoleHost {
		r.queue = append(r.queue, p)
		return true
	}
	// A host waits behind other hosts but ahead of every member — "the admin is
	// always first in line" from the task, without letting one host jump another.
	at := len(r.queue)
	for i, q := range r.queue {
		if q.Role != RoleHost {
			at = i
			break
		}
	}
	r.queue = append(r.queue, nil)
	copy(r.queue[at+1:], r.queue[at:])
	r.queue[at] = p
	return true
}

// dequeueLocked drops p from the waiting line, reporting whether it was there.
// Caller holds r.mu.
func (r *Room) dequeueLocked(p *Participant) bool {
	for i, q := range r.queue {
		if q == p {
			r.queue = append(r.queue[:i], r.queue[i+1:]...)
			return true
		}
	}
	return false
}

// expireStageLocked drops an abandoned stage and promotes the next presenter,
// reporting whether anything changed. Caller holds r.mu.
func (r *Room) expireStageLocked(now time.Time) bool {
	if r.stage == nil || !now.After(r.stage.expires) {
		return false
	}
	r.promoteLocked(now)
	return true
}

// sweep expires an abandoned stage and forgets elapsed kick cooldowns.
func (r *Room) sweep(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for uid, until := range r.kicked {
		if !until.After(now) {
			delete(r.kicked, uid)
		}
	}
	if r.expireStageLocked(now) {
		r.broadcastLocked()
	}
}

// State returns the current snapshot (used by tests and by the join path).
func (r *Room) State(now time.Time) StateMsg {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireStageLocked(now)
	return r.stateLocked()
}

// stateLocked builds the snapshot; caller holds r.mu. The participant list is
// sorted so the payload is stable across broadcasts — map iteration order would
// otherwise make every snapshot look like a change to a diffing client. The
// queue keeps its own order, which *is* the information it carries.
func (r *Room) stateLocked() StateMsg {
	byUser := map[uuid.UUID]*PersonView{}
	for p := range r.members {
		v, ok := byUser[p.UserID]
		if !ok {
			v = &PersonView{
				UserID:     p.UserID.String(),
				Name:       p.Name,
				Role:       p.Role,
				ForceMuted: r.muted[p.UserID],
				JoinedAt:   p.joinedAt,
			}
			byUser[p.UserID] = v
		}
		v.Conns++
		v.Mic = v.Mic || p.mic
		v.Cam = v.Cam || p.cam
		// A host in any tab is a host in the room, and the earliest raise wins so
		// the speaking order does not shuffle when a second tab opens.
		if p.Role == RoleHost {
			v.Role = RoleHost
		}
		if p.joinedAt.Before(v.JoinedAt) {
			v.JoinedAt = p.joinedAt
		}
		if !p.handAt.IsZero() && (v.HandAt == nil || p.handAt.Before(*v.HandAt)) {
			at := p.handAt
			v.HandAt = &at
		}
	}
	people := make([]PersonView, 0, len(byUser))
	for _, v := range byUser {
		people = append(people, *v)
	}
	sort.Slice(people, func(i, j int) bool {
		if !people[i].JoinedAt.Equal(people[j].JoinedAt) {
			return people[i].JoinedAt.Before(people[j].JoinedAt)
		}
		return people[i].UserID < people[j].UserID
	})
	queue := make([]QueueView, 0, len(r.queue))
	for _, q := range r.queue {
		queue = append(queue, QueueView{
			UserID: q.UserID.String(), ConnID: q.ID.String(), Name: q.Name, Role: q.Role,
		})
	}
	msg := StateMsg{Type: TypeState, Participants: people, Queue: queue}
	if r.stage != nil {
		msg.Stage = &StageView{
			UserID: r.stage.p.UserID.String(),
			ConnID: r.stage.p.ID.String(),
			Name:   r.stage.p.Name,
			Since:  r.stage.since,
		}
	}
	return msg
}

// broadcastLocked sends the snapshot to everyone; caller holds r.mu.
func (r *Room) broadcastLocked() {
	frame := encode(r.stateLocked())
	for p := range r.members {
		p.deliver(frame)
	}
}

// notify sends a payload-free "go and read it" frame to everyone in the room —
// used for the chat (#2873), whose messages live in the database and are
// fetched over HTTP. Keeping the body out of the frame is what keeps the
// eviction contract above safe: a chat message with an attachment would
// otherwise be able to evict the people it is addressed to.
func (r *Room) notify(msgType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	frame := encode(struct {
		Type string `json:"type"`
	}{msgType})
	for p := range r.members {
		p.deliver(frame)
	}
}

// Size reports the number of open sockets in this conference.
func (r *Room) Size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.members)
}

func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// Every message type here is a plain struct of strings, bools and times.
		log.Printf("confroom: encode %T: %v", v, err)
		return []byte(`{"type":"state","participants":[],"stage":null,"queue":[]}`)
	}
	return b
}

// Rooms is the registry of live conference rooms. A room exists only while
// someone is in it — with one exception: a kick cooldown keeps an otherwise
// empty room alive until it elapses, because forgetting the room is the same as
// forgetting the kick.
type Rooms struct {
	mu      sync.Mutex
	rooms   map[uuid.UUID]*Room
	enforce Enforcer
	done    chan struct{}
	once    sync.Once
}

// New returns an empty registry. Call Run once to start the sweeper.
func New() *Rooms {
	return &Rooms{rooms: map[uuid.UUID]*Room{}, done: make(chan struct{})}
}

// SetEnforcer installs the sink for moderation decisions (#2872). Call it once
// during wiring, before any room exists: rooms copy the enforcer as they are
// created, so one set afterwards would only reach conferences opened later.
func (rs *Rooms) SetEnforcer(e Enforcer) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.enforce = e
}

// KickedUntil reports whether a user is still serving a kick cooldown for a
// conference. The socket handler checks this *before* upgrading, so a kicked
// client gets a real status code instead of a connection that closes itself.
func (rs *Rooms) KickedUntil(confID, userID uuid.UUID) (time.Time, bool) {
	rs.mu.Lock()
	room, ok := rs.rooms[confID]
	rs.mu.Unlock()
	if !ok {
		return time.Time{}, false
	}
	return room.kickedUntil(userID, time.Now())
}

// Join puts a participant into a conference's room, creating it if needed.
// forceMuted is the flag from their participant row — see Room.join for why the
// caller has to supply it rather than the room remembering.
func (rs *Rooms) Join(confID uuid.UUID, p *Participant, forceMuted bool) *Room {
	rs.mu.Lock()
	room, ok := rs.rooms[confID]
	if !ok {
		room = newRoom(confID, rs.enforce)
		rs.rooms[confID] = room
	}
	rs.mu.Unlock()
	room.join(p, forceMuted, time.Now())
	return room
}

// Leave removes a participant and forgets the room once it is empty and holds
// no live cooldown.
func (rs *Rooms) Leave(confID uuid.UUID, p *Participant) {
	rs.mu.Lock()
	room, ok := rs.rooms[confID]
	rs.mu.Unlock()
	if !ok {
		return
	}
	if room.leave(p, time.Now()) {
		rs.mu.Lock()
		// Re-check under the lock: someone may have joined between leave() and
		// here, and deleting the room then would strand them in an entry nobody
		// else can find.
		if room.Size() == 0 && rs.rooms[confID] == room && !room.hasCooldown(time.Now()) {
			delete(rs.rooms, confID)
		}
		rs.mu.Unlock()
	}
}

// hasCooldown reports whether any kick is still in force.
func (r *Room) hasCooldown(now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, until := range r.kicked {
		if until.After(now) {
			return true
		}
	}
	return false
}

// Drop tells everyone in a conference that it is over and closes the room.
// Called when the call is ended or deleted: without it participants would sit
// in a room whose conference no longer exists and only find out when their next
// request 404s.
func (rs *Rooms) Drop(confID uuid.UUID, reason string) {
	rs.mu.Lock()
	room, ok := rs.rooms[confID]
	delete(rs.rooms, confID)
	rs.mu.Unlock()
	if !ok {
		return
	}
	frame := encode(struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	}{TypeEnded, reason})
	room.mu.Lock()
	for p := range room.members {
		delete(room.members, p)
		// Queue the reason, then close: a closed channel still yields what was
		// buffered, so the write pump delivers the frame before it finishes.
		p.deliver(frame)
		p.Close()
	}
	room.stage = nil
	room.queue = nil
	room.mu.Unlock()
}

// Notify tells everyone currently in a conference that something they are
// looking at changed outside the socket (a chat message, #2873). A conference
// nobody has open has no room and the call is a no-op — which is correct: the
// next arrival loads the current state over HTTP anyway.
func (rs *Rooms) Notify(confID uuid.UUID, msgType string) {
	rs.mu.Lock()
	room, ok := rs.rooms[confID]
	rs.mu.Unlock()
	if !ok {
		return
	}
	room.notify(msgType)
}

// Run sweeps abandoned stages and elapsed cooldowns until Close. Call it once,
// in its own goroutine.
func (rs *Rooms) Run() {
	ticker := time.NewTicker(SweepEvery)
	defer ticker.Stop()
	for {
		select {
		case <-rs.done:
			return
		case now := <-ticker.C:
			rs.SweepAll(now)
		}
	}
}

// SweepAll expires stages and cooldowns in every room as of now, and forgets
// rooms that are left with neither participants nor a live cooldown.
func (rs *Rooms) SweepAll(now time.Time) {
	rs.mu.Lock()
	rooms := make(map[uuid.UUID]*Room, len(rs.rooms))
	for id, r := range rs.rooms {
		rooms[id] = r
	}
	rs.mu.Unlock()
	for id, r := range rooms {
		r.sweep(now)
		if r.Size() > 0 || r.hasCooldown(now) {
			continue
		}
		// An empty room only survives for its cooldowns; once they elapse it is
		// a leak — one entry per conference ever opened.
		rs.mu.Lock()
		if rs.rooms[id] == r && r.Size() == 0 && !r.hasCooldown(now) {
			delete(rs.rooms, id)
		}
		rs.mu.Unlock()
	}
}

// Close stops Run and disconnects everyone, so clients reconnect to the
// replacement process instead of waiting on a stage nobody can release.
func (rs *Rooms) Close() {
	rs.once.Do(func() { close(rs.done) })
	rs.mu.Lock()
	rooms := rs.rooms
	rs.rooms = map[uuid.UUID]*Room{}
	rs.mu.Unlock()
	for _, r := range rooms {
		r.mu.Lock()
		for p := range r.members {
			delete(r.members, p)
			p.Close()
		}
		r.mu.Unlock()
	}
}

// UserPresent reports whether a user still has at least one live connection in a
// conference. The socket handler checks this after a disconnect to tell "closed
// one of two tabs" (still in the call) from "left the call" (reconcile the DB).
func (rs *Rooms) UserPresent(confID, userID uuid.UUID) bool {
	rs.mu.Lock()
	room, ok := rs.rooms[confID]
	rs.mu.Unlock()
	if !ok {
		return false
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	for m := range room.members {
		if m.UserID == userID {
			return true
		}
	}
	return false
}

// Count reports how many conferences currently have a live room.
func (rs *Rooms) Count() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.rooms)
}
