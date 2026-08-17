// Package docroom is the per-document collaboration channel: who is looking at
// a document right now, and who holds which block while they type in it.
//
// It is deliberately separate from internal/realtime's workspace hub, for two
// reasons that are easy to lose sight of later:
//
//   - the hub is one-way. Its clients are read-only consumers of broadcasts
//     (realtime/client.go: the read pump exists only to notice pongs and
//     disconnects), while a lock has to be *asked* for and answered;
//   - the hub is workspace-scoped. Presence and lock traffic for one document
//     would fan out to every member of the workspace, most of whom have that
//     document closed.
//
// Delivery here is also stricter than the hub's. The hub drops an event for a
// slow client and flags a resync (hub.go:107-113); this package never drops a
// message silently — a participant whose buffer overflows is evicted, and its
// reconnect starts from a fresh snapshot. That matters more than it looks: every
// broadcast below carries the *whole* room state rather than a delta, so a
// reconnect is self-healing, whereas a silently skipped frame would leave a
// block shown as locked by someone who left ten minutes ago.
package docroom

import (
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// LockTTL is how long a block lock survives without a refresh. The holder's
	// client re-sends its lock while the caret stays in the block, so the TTL
	// only comes into play when a client dies without closing its socket (laptop
	// lid, killed tab, dropped Wi-Fi). Short enough that a colleague is not
	// staring at a stale "editing" badge for long; long enough to survive a
	// refresh interval plus a slow round trip.
	LockTTL = 30 * time.Second

	// SweepEvery is how often expired locks are collected. Locks are also checked
	// lazily on every request, so this only bounds how long an *unobserved* stale
	// lock stays visible to other participants.
	SweepEvery = 5 * time.Second

	// sendBuffer is how many messages may queue for one participant before it is
	// evicted as too slow. State snapshots are small and idempotent, so a client
	// that cannot keep up with this is not a client we can usefully talk to.
	sendBuffer = 16
)

// Message types on the wire. Server→client: welcome (once, on join), state
// (whole-room snapshot), denied (answer to a lock request that lost), comments
// (something changed in this document's threads — refetch them), content (the
// body was replaced under the readers' feet — reload it), links (its task links
// or approval protocols changed).
// Client→server: lock, unlock.
const (
	TypeWelcome  = "welcome"
	TypeState    = "state"
	TypeDenied   = "denied"
	TypeComments = "comments"
	// TypeContent is sent when the document's body was rewritten by something
	// other than an edit in the room — today only a rollback to an earlier
	// version (#2731). It carries no payload: everyone refetches, which is also
	// what makes a nudge lost to a reconnect cost nothing but a stale tab.
	TypeContent = "content"
	// TypeLinks is sent when the document's task links or approval protocols
	// changed (#2732). Payload-free like the others: the panel refetches, which
	// is what keeps a nudge lost to a reconnect from turning into a phantom row.
	TypeLinks = "links"
	// TypeContentSaved is sent after someone in the room saved the body, so the
	// others stop reading a document that has moved on under them (#2729 rework).
	//
	// It is deliberately *not* TypeContent: that one means a rollback and the
	// client answers it with a hard reload and an announcement, which is the right
	// response to "the body was replaced from history" and an absurd one to
	// "a colleague typed a word".
	//
	// The frame carries a timestamp, not the document. The room evicts a
	// participant whose buffer fills (see the package comment), and that contract
	// holds only while frames are small: a few hundred kilobytes of content, times
	// an autosave every 0.8s, times every reader, turns this channel into a machine
	// for evicting the very people it exists to inform. So the frame is a nudge and
	// the body is refetched over HTTP.
	TypeContentSaved = "content.saved"
	TypeLock         = "lock"
	TypeUnlock       = "unlock"
)

// Participant is one open socket on one document. The same user may have two
// tabs open, so identity here is the connection id, not the user id — otherwise
// the second tab would silently inherit the first tab's locks.
type Participant struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Name   string

	send   chan []byte
	closed chan struct{}
	once   sync.Once
}

// NewParticipant returns a participant ready to be joined to a room.
func NewParticipant(userID uuid.UUID, name string) *Participant {
	return &Participant{
		ID:     uuid.New(),
		UserID: userID,
		Name:   name,
		send:   make(chan []byte, sendBuffer),
		closed: make(chan struct{}),
	}
}

// Out is the stream of frames to write to the socket. It is closed when the
// participant is evicted or leaves, which is the write pump's signal to finish.
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
		// Full buffer means the socket is not draining. Dropping the frame would
		// leave this client's view of who-holds-what frozen; closing it makes the
		// client reconnect and re-read the room.
		log.Printf("docroom: evicting slow participant %s", p.ID)
		p.Close()
	}
}

// lock is one held block.
type lock struct {
	conn    uuid.UUID
	userID  uuid.UUID
	name    string
	expires time.Time
}

// Room is the live state of one document.
type Room struct {
	docID uuid.UUID

	mu      sync.Mutex
	members map[*Participant]struct{}
	locks   map[string]lock
}

func newRoom(docID uuid.UUID) *Room {
	return &Room{docID: docID, members: map[*Participant]struct{}{}, locks: map[string]lock{}}
}

// ViewerView is one person in the room. Conns counts that person's open sockets
// so the UI can show a single avatar for someone with the document open twice.
type ViewerView struct {
	UserID string   `json:"user_id"`
	Name   string   `json:"name"`
	Conns  int      `json:"conns"`
	Blocks []string `json:"blocks"`
}

// LockView is one held block as seen by clients.
type LockView struct {
	BlockID string `json:"block_id"`
	UserID  string `json:"user_id"`
	ConnID  string `json:"conn_id"`
	Name    string `json:"name"`
}

// StateMsg is the whole-room snapshot. Sending the whole state on every change
// (rather than add/remove deltas) is what makes a reconnect enough to recover.
type StateMsg struct {
	Type    string       `json:"type"`
	Viewers []ViewerView `json:"viewers"`
	Locks   []LockView   `json:"locks"`
}

// welcomeMsg tells a fresh participant which connection it is. A client needs
// this to tell "I hold this block" from "my other tab holds this block", which
// is the difference between typing and being locked out.
type welcomeMsg struct {
	Type      string `json:"type"`
	ConnID    string `json:"conn_id"`
	UserID    string `json:"user_id"`
	LockTTLMs int64  `json:"lock_ttl_ms"`
}

// deniedMsg answers a lock request that lost the race, naming the holder so the
// loser can say who is in the block rather than just refusing to type.
type deniedMsg struct {
	Type    string `json:"type"`
	BlockID string `json:"block_id"`
	UserID  string `json:"user_id"`
	Name    string `json:"name"`
}

// ContentSavedMsg announces that the document body changed, without carrying it.
//
// ByConn is who saved it, and it is a *connection* rather than a user on
// purpose: a person with the document open in two tabs has two carets, and the
// second one needs the edit as much as a stranger's would. Excluding by user id
// would leave that tab silently stale.
type ContentSavedMsg struct {
	Type      string    `json:"type"`
	UpdatedAt time.Time `json:"updated_at"`
	ByConn    string    `json:"by_conn"`
	ByUser    string    `json:"by_user"`
}

// join adds a participant, sends it a welcome and broadcasts the new state.
func (r *Room) join(p *Participant, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members[p] = struct{}{}
	p.deliver(encode(welcomeMsg{
		Type:      TypeWelcome,
		ConnID:    p.ID.String(),
		UserID:    p.UserID.String(),
		LockTTLMs: LockTTL.Milliseconds(),
	}))
	r.broadcastLocked(now)
}

// leave removes a participant and drops every lock it held. Releasing on
// disconnect (rather than waiting for the TTL) is what makes closing a tab feel
// immediate to everyone else.
func (r *Room) leave(p *Participant, now time.Time) (empty bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.members[p]; !ok {
		return len(r.members) == 0
	}
	delete(r.members, p)
	for blockID, l := range r.locks {
		if l.conn == p.ID {
			delete(r.locks, blockID)
		}
	}
	p.Close()
	r.broadcastLocked(now)
	return len(r.members) == 0
}

// Lock grants blockID to p, or refuses when someone else holds it. Re-locking a
// block you already hold is the refresh path and always succeeds.
func (r *Room) Lock(p *Participant, blockID string, now time.Time) bool {
	if blockID == "" {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(now)
	if held, ok := r.locks[blockID]; ok && held.conn != p.ID {
		p.deliver(encode(deniedMsg{
			Type: TypeDenied, BlockID: blockID,
			UserID: held.userID.String(), Name: held.name,
		}))
		return false
	}
	// One caret per participant: taking a new block releases the previous one.
	// Without this, walking down a page with the arrow keys would leave a trail
	// of blocks locked for the next 30 seconds.
	for id, l := range r.locks {
		if l.conn == p.ID && id != blockID {
			delete(r.locks, id)
		}
	}
	prev, existed := r.locks[blockID]
	r.locks[blockID] = lock{conn: p.ID, userID: p.UserID, name: p.Name, expires: now.Add(LockTTL)}
	// A pure refresh changes nothing anyone else can see; staying quiet keeps the
	// heartbeat from broadcasting to the whole room every few seconds.
	if !existed || prev.conn != p.ID {
		r.broadcastLocked(now)
	}
	return true
}

// Unlock releases a block held by p. Releasing someone else's lock is not an
// error, it is simply ignored — a stale client should not be able to unlock a
// block another person is typing in.
func (r *Room) Unlock(p *Participant, blockID string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.locks[blockID]; !ok || l.conn != p.ID {
		return
	}
	delete(r.locks, blockID)
	r.broadcastLocked(now)
}

// sweep drops expired locks and broadcasts if anything changed.
func (r *Room) sweep(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.expireLocked(now) {
		r.broadcastLocked(now)
	}
}

// expireLocked removes timed-out locks; caller holds r.mu.
func (r *Room) expireLocked(now time.Time) bool {
	changed := false
	for blockID, l := range r.locks {
		if now.After(l.expires) {
			delete(r.locks, blockID)
			changed = true
		}
	}
	return changed
}

// State returns the current snapshot (used by tests and by the join path).
func (r *Room) State(now time.Time) StateMsg {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(now)
	return r.stateLocked()
}

// stateLocked builds the snapshot; caller holds r.mu. Both lists are sorted so
// the payload is stable across broadcasts — map iteration order would otherwise
// make every snapshot look like a change to a diffing client.
func (r *Room) stateLocked() StateMsg {
	byUser := map[uuid.UUID]*ViewerView{}
	for p := range r.members {
		v, ok := byUser[p.UserID]
		if !ok {
			v = &ViewerView{UserID: p.UserID.String(), Name: p.Name, Blocks: []string{}}
			byUser[p.UserID] = v
		}
		v.Conns++
	}
	locks := make([]LockView, 0, len(r.locks))
	for blockID, l := range r.locks {
		locks = append(locks, LockView{
			BlockID: blockID, UserID: l.userID.String(),
			ConnID: l.conn.String(), Name: l.name,
		})
		if v, ok := byUser[l.userID]; ok {
			v.Blocks = append(v.Blocks, blockID)
		}
	}
	viewers := make([]ViewerView, 0, len(byUser))
	for _, v := range byUser {
		sort.Strings(v.Blocks)
		viewers = append(viewers, *v)
	}
	sort.Slice(viewers, func(i, j int) bool { return viewers[i].UserID < viewers[j].UserID })
	sort.Slice(locks, func(i, j int) bool { return locks[i].BlockID < locks[j].BlockID })
	return StateMsg{Type: TypeState, Viewers: viewers, Locks: locks}
}

// broadcastLocked sends the snapshot to everyone; caller holds r.mu.
func (r *Room) broadcastLocked(_ time.Time) {
	frame := encode(r.stateLocked())
	for p := range r.members {
		p.deliver(frame)
	}
}

// notify sends a payload-free frame of the given type to everyone in the room.
// Used for "something changed, go and read it" nudges (#2730) — the room itself
// holds no such state, so there is nothing to put in the frame.
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

// send delivers an arbitrary payload to everyone in the room except the
// connection named by exceptConn (uuid.Nil excludes nobody). Unlike notify it
// carries data, so the payload has to stay small — see TypeContentSaved.
func (r *Room) send(v any, exceptConn uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	frame := encode(v)
	for p := range r.members {
		if p.ID == exceptConn {
			continue
		}
		p.deliver(frame)
	}
}

// Size reports the number of open sockets on this document.
func (r *Room) Size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.members)
}

func encode(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// Every message type here is a plain struct of strings and ints.
		log.Printf("docroom: encode %T: %v", v, err)
		return []byte(`{"type":"state","viewers":[],"locks":[]}`)
	}
	return b
}

// Rooms is the registry of live document rooms. A room exists only while at
// least one participant is in it: documents are long-lived and numerous, and
// keeping an entry per document ever opened would be a slow leak.
type Rooms struct {
	mu    sync.Mutex
	rooms map[uuid.UUID]*Room
	done  chan struct{}
	once  sync.Once
}

// New returns an empty registry. Call Run once to start lock expiry.
func New() *Rooms {
	return &Rooms{rooms: map[uuid.UUID]*Room{}, done: make(chan struct{})}
}

// Join puts a participant into a document's room, creating it if needed.
func (rs *Rooms) Join(docID uuid.UUID, p *Participant) *Room {
	rs.mu.Lock()
	room, ok := rs.rooms[docID]
	if !ok {
		room = newRoom(docID)
		rs.rooms[docID] = room
	}
	rs.mu.Unlock()
	room.join(p, time.Now())
	return room
}

// Leave removes a participant and forgets the room once it is empty.
func (rs *Rooms) Leave(docID uuid.UUID, p *Participant) {
	rs.mu.Lock()
	room, ok := rs.rooms[docID]
	rs.mu.Unlock()
	if !ok {
		return
	}
	if room.leave(p, time.Now()) {
		rs.mu.Lock()
		// Re-check under the lock: someone may have joined between leave() and
		// here, and deleting the room then would strand them in an entry nobody
		// else can find.
		if room.Size() == 0 && rs.rooms[docID] == room {
			delete(rs.rooms, docID)
		}
		rs.mu.Unlock()
	}
}

// Drop disconnects everyone in a document's room and forgets it. Called when the
// document is deleted: without it the remaining participants would keep typing
// into a document that no longer exists and only find out on the next save.
func (rs *Rooms) Drop(docID uuid.UUID) {
	rs.mu.Lock()
	room, ok := rs.rooms[docID]
	delete(rs.rooms, docID)
	rs.mu.Unlock()
	if !ok {
		return
	}
	room.mu.Lock()
	for p := range room.members {
		delete(room.members, p)
		p.Close()
	}
	room.mu.Unlock()
}

// Notify tells everyone currently in a document's room that something they are
// looking at changed outside the socket (its comment threads, #2730). A document
// nobody has open has no room, and the call is then a no-op — which is correct:
// the next reader loads the current state over HTTP anyway.
func (rs *Rooms) Notify(docID uuid.UUID, msgType string) {
	rs.mu.Lock()
	room, ok := rs.rooms[docID]
	rs.mu.Unlock()
	if !ok {
		return
	}
	room.notify(msgType)
}

// Send delivers a payload-carrying frame to a document's room, skipping the
// connection that caused it. A document nobody has open has no room and the call
// is a no-op, which is correct: the next reader loads the body over HTTP anyway.
func (rs *Rooms) Send(docID uuid.UUID, exceptConn uuid.UUID, v any) {
	rs.mu.Lock()
	room, ok := rs.rooms[docID]
	rs.mu.Unlock()
	if !ok {
		return
	}
	room.send(v, exceptConn)
}

// Run expires abandoned locks until Close. Call it once, in its own goroutine.
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

// SweepAll expires locks in every room as of now.
func (rs *Rooms) SweepAll(now time.Time) {
	rs.mu.Lock()
	rooms := make([]*Room, 0, len(rs.rooms))
	for _, r := range rs.rooms {
		rooms = append(rooms, r)
	}
	rs.mu.Unlock()
	for _, r := range rooms {
		r.sweep(now)
	}
}

// Close stops Run and disconnects every participant, so clients reconnect to the
// replacement process instead of holding locks nobody can release.
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

// Count reports how many documents currently have someone in them.
func (rs *Rooms) Count() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.rooms)
}
