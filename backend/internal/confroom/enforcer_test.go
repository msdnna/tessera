package confroom

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// recorder is a stand-in Enforcer. Mutex-guarded because the room calls it from
// whichever goroutine issued the command, and the assertions run on the test's.
type recorder struct {
	mu     sync.Mutex
	kicks  []uuid.UUID
	mutes  []muteCall
	inLock func() // called while the enforcer runs, to probe the room's lock
}

type muteCall struct {
	user  uuid.UUID
	muted bool
}

func (r *recorder) Kicked(_, userID uuid.UUID) {
	if r.inLock != nil {
		r.inLock()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kicks = append(r.kicks, userID)
}

func (r *recorder) Muted(_, userID uuid.UUID, muted bool) {
	if r.inLock != nil {
		r.inLock()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mutes = append(r.mutes, muteCall{userID, muted})
}

func (r *recorder) snapshot() ([]uuid.UUID, []muteCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]uuid.UUID(nil), r.kicks...), append([]muteCall(nil), r.mutes...)
}

// TestModerationReachesTheEnforcer is the contract #2872 rests on: a decision
// taken in the room has to leave the process, or the force-mute flag is a badge
// on a microphone that is still publishing and the kick is undone by pressing
// join again.
func TestModerationReachesTheEnforcer(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	rs, confID := New(), uuid.New()
	rs.SetEnforcer(rec)
	host := join(rs, confID, "Хост", RoleHost)
	member := join(rs, confID, "Ира", RoleMember)
	room := rs.rooms[confID]
	now := time.Now()

	if !room.SetMuted(host, member.UserID, true) {
		t.Fatal("the host was refused a force-mute")
	}
	if !room.Kick(host, member.UserID, now) {
		t.Fatal("the host was refused a kick")
	}

	kicks, mutes := rec.snapshot()
	if len(kicks) != 1 || kicks[0] != member.UserID {
		t.Errorf("kicks = %v, want one entry for the member", kicks)
	}
	if len(mutes) != 1 || mutes[0] != (muteCall{member.UserID, true}) {
		t.Errorf("mutes = %v, want one force-mute of the member", mutes)
	}
}

// TestRefusedModerationDoesNotReachTheEnforcer: a member's kick is denied in the
// room, and the enforcer is where a decision becomes a database write and an SFU
// call — letting a refusal through would make the check decorative.
func TestRefusedModerationDoesNotReachTheEnforcer(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	rs, confID := New(), uuid.New()
	rs.SetEnforcer(rec)
	host := join(rs, confID, "Хост", RoleHost)
	member := join(rs, confID, "Ира", RoleMember)
	room := rs.rooms[confID]

	room.Kick(member, host.UserID, time.Now())
	room.SetMuted(member, host.UserID, true)

	if kicks, mutes := rec.snapshot(); len(kicks) != 0 || len(mutes) != 0 {
		t.Errorf("a member's refused commands were enforced anyway: %v %v", kicks, mutes)
	}
}

// TestUnchangedMuteIsNotReEnforced: repeating a force-mute is idempotent in the
// room, and it has to stay idempotent outside it too. Each call costs a
// ListParticipants plus a mute per track at the SFU, and a UI that re-sends the
// state it already sees would turn a toggle into a stream of them.
func TestUnchangedMuteIsNotReEnforced(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	rs, confID := New(), uuid.New()
	rs.SetEnforcer(rec)
	host := join(rs, confID, "Хост", RoleHost)
	member := join(rs, confID, "Ира", RoleMember)
	room := rs.rooms[confID]

	room.SetMuted(host, member.UserID, true)
	room.SetMuted(host, member.UserID, true)
	room.SetMuted(host, member.UserID, false)

	_, mutes := rec.snapshot()
	want := []muteCall{{member.UserID, true}, {member.UserID, false}}
	if len(mutes) != len(want) || mutes[0] != want[0] || mutes[1] != want[1] {
		t.Errorf("mutes = %v, want %v", mutes, want)
	}
}

// TestEnforcerRunsOutsideTheRoomLock. The enforcer talks to Postgres and to the
// SFU; if it were called with the room's mutex held, one slow round trip would
// stall every other participant's snapshot behind it — and an implementation
// that read the room back would deadlock outright.
func TestEnforcerRunsOutsideTheRoomLock(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	var room *Room
	rec := &recorder{inLock: func() {
		// Size takes the same mutex. Held by the caller, this would never return.
		room.Size()
	}}
	rs.SetEnforcer(rec)
	host := join(rs, confID, "Хост", RoleHost)
	member := join(rs, confID, "Ира", RoleMember)
	room = rs.rooms[confID]

	done := make(chan struct{})
	go func() {
		defer close(done)
		room.SetMuted(host, member.UserID, true)
		room.Kick(host, member.UserID, time.Now())
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("moderation deadlocked: the enforcer is called under the room lock")
	}
}

// TestForceMuteSurvivesAnEmptyRoom. The flag lives in memory and dies with the
// room, so the socket seeds it from the participant row on join. Without that,
// the way to get your microphone back after a force-mute would be to wait for
// the call to empty and be the first one back in.
func TestForceMuteSurvivesAnEmptyRoom(t *testing.T) {
	t.Parallel()
	rs, confID := New(), uuid.New()
	muted := NewParticipant(uuid.New(), "Ира", RoleMember)
	room := rs.Join(confID, muted, true)

	if !room.Muted(muted.UserID) {
		t.Fatal("the stored force-mute was not applied on join")
	}
	// And it is honoured, not merely recorded: a client reporting a live
	// microphone must not be able to talk its way out of the mute.
	room.SetMedia(muted, true, true)
	people := room.State(time.Now()).Participants
	if len(people) != 1 {
		t.Fatalf("participants = %d, want 1", len(people))
	}
	if people[0].Mic {
		t.Error("a force-muted participant reported a live microphone")
	}
	if !people[0].ForceMuted {
		t.Error("the roster does not show the participant as force-muted")
	}
}
